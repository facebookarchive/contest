// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/tools/migration/rdbms/migrate"

	"github.com/facebookincubator/contest/cmds/plugins"
	"github.com/facebookincubator/contest/pkg/pluginregistry"

	"github.com/sirupsen/logrus"
)

const shardSize = uint64(50)

// Core Contest data structures for migration from v1 to v2. These data structures
// have been migrated in the core framework so they need to be preserved in the
// migration package.

// Request represent the v1 job request layout
type Request struct {
	JobID         types.JobID
	JobName       string
	Requestor     string
	ServerID      string
	RequestTime   time.Time
	JobDescriptor string
	// TestDescriptors are the fetched test steps as per the test fetcher
	// defined in the JobDescriptor above.
	TestDescriptors string
}

// DescriptorMigration represents a migration which moves steps description in jobs tables from old to new
// schema. The migration consists in the following:
//
//
// In v0001, Request object was structured as follows:
//
// type Request struct {
//		JobID         types.JobID
//		JobName       string
//		Requestor     string
//		ServerID      string
//		RequestTime   time.Time
//		JobDescriptor string
//		TestDescriptors string
// }
//
// Having TestDescriptors as a field of request objects creates several issues, including:
// * It's an abstraction leakage, the extended description of the steps might not be part
//   of the initial request submitted from outside (i.e. might not be a literal embedded
//	 in the job descriptor)
// * The job.Descriptor object does not contain the extended version of the test steps, which
//   will make it difficult to handle resume, as we need to retrieve multiple objects
//
// Schema v0002 introduces the concept of extended_descriptor, which is defined as follows:
// type ExtendedDescriptor struct {
//		JobDescriptor
//		StepsDescriptors []test.StepsDescriptors
// }
//
// We remove TestDescriptors from Request objects, and we store that information side-by-side with
// JobDescriptor into an ExtendedDescriptor. We then store this ExtendedDescriptor in the jobs table
// so that all the test information can be re-fetched simply by reading extended_descriptor field in
// jobs table.
type DescriptorMigration struct {
	log *logrus.Entry
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

// fetchJobs fetches job requests based on limit and offset
func fetchJobs(tx *sql.Tx, limit, offset uint64, log *logrus.Entry) ([]Request, error) {

	log.Debugf("fetching shard limit: %d, offset: %d", limit, offset)
	selectStatement := "select job_id, name, requestor, server_id, request_time, descriptor, teststeps from jobs limit ? offset ?"
	log.Debugf("running query: %s", selectStatement)

	start := time.Now()
	rows, err := tx.Query(selectStatement, limit, offset)

	elapsed := time.Since(start)
	log.Debugf("select query executed in: %.3f ms", ms(elapsed))

	if err != nil {
		return nil, fmt.Errorf("could not get job request (limit %d, offset %d): %w", limit, offset, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var jobs []Request

	start = time.Now()
	for rows.Next() {
		job := Request{}
		err := rows.Scan(
			&job.JobID,
			&job.JobName,
			&job.Requestor,
			&job.ServerID,
			&job.RequestTime,
			&job.JobDescriptor,
			&job.TestDescriptors,
		)
		if err != nil {
			return nil, fmt.Errorf("could not scan job request (limit %d, offset %d): %w", limit, offset, err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not scan job request (limit %d, offset %d): %w", limit, offset, err)
	}

	if len(jobs) == 0 {
		return nil, fmt.Errorf("could not find jobs for limit: %d, offset: %d", limit, offset)
	}
	elapsed = time.Since(start)
	log.Debugf("jobs in shard shard limit: %d, offset: %d fetched in: %.3f ms", limit, offset, ms(elapsed))
	return jobs, nil
}

func migrateJobs(tx *sql.Tx, requests []Request, registry *pluginregistry.PluginRegistry, log *logrus.Entry) error {

	log.Debugf("migrating %d jobs", len(requests))
	start := time.Now()
	for _, request := range requests {

		// Merge JobDescriptor and TestStepDescriptor(s) into a single ExtendedDescriptor.
		// ExtendedDescriptor contains StepsDescriptors, which is declared as follows:
		// type StepsDescriptors struct {
		//		TestName string
		// 		Test     []StepDescriptor
		// 		Cleanup  []StepDescriptor
		// }
		//
		// StepDescriptor is instead defined as follows:
		// type StepDescriptor struct {
		//		Name       string
		//		Label      string
		//		Parameters StepParameters
		// }
		//
		// The previous request.TestDescriptors was actually referring to the JSON
		// description representing the steps, for every test. This is very ambiguous
		// because job.JobDescriptor contains TestDescriptors itself, which however
		// represent the higher level description of the test (including TargetManager
		// name, TestFetcher name, etc.). So ExtendedDescriptor refers instead to
		// StepsDescriptors and TestDescriptors field is removed from request object.

		var jobDesc job.Descriptor
		if err := json.Unmarshal([]byte(request.JobDescriptor), &jobDesc); err != nil {
			return fmt.Errorf("failed to unmarshal job descriptor (%+v): %w", jobDesc, err)
		}

		var stepDescs [][]*test.StepDescriptor
		if err := json.Unmarshal([]byte(request.TestDescriptors), &stepDescs); err != nil {
			return fmt.Errorf("failed to unmarshal test step descriptors from request object (%+v): %w", request.TestDescriptors, err)
		}

		extendedDescriptor := job.ExtendedDescriptor{Descriptor: jobDesc}
		if len(stepDescs) != len(jobDesc.TestDescriptors) {
			return fmt.Errorf("number of tests described in JobDescriptor does not match steps stored in db")
		}

		for index, stepDesc := range stepDescs {
			newStepsDesc := test.StepsDescriptors{}
			// TestName is missing from the v0001 schema and can be retrieved only via
			// TestFetcher. We need TestName in the extended_descriptor to be able to
			// correctly build status of previous jobs. So, the only option we have is to
			// initialize a TestFetcher and let it retrieve the test name.

			for _, desc := range stepDesc {
				newStepsDesc.Test = append(newStepsDesc.Test, *desc)
			}

			// Look up the original TestDescriptor from JobDescriptor, instantiate
			// TestFetcher accordingly and retrieve the name of the test
			td := jobDesc.TestDescriptors[index]

			tfb, err := registry.NewTestFetcherBundle(&td)
			if err != nil {
				return fmt.Errorf("could not instantiate test fetcher for jobID %d based on descriptor %+v: %w", request.JobID, td, err)
			}

			stepsDescriptors, err := tfb.TestFetcher.Fetch(tfb.FetchParameters)
			if err != nil {
				return fmt.Errorf("could not retrieve test description from fetcher for jobID %d: %w", request.JobID, err)
			}

			// Check that the serialization of the steps retrieved by the test fetcher matches the steps
			// stored in the DB. If that's not the case, then, just print a warning: the underlying test
			/// might have changed.We go ahead anyway assuming assume the test name is still relevant.
			stepDescFetchedJSON, err := json.Marshal(stepsDescriptors.Test)
			if err != nil {
				log.Warningf("steps description (`%v`) fetched by test fetcher for job %d cannot be serialized: %v", stepsDescriptors.Test, request.JobID, err)
			}

			stepDescDBJSON, err := json.Marshal(stepDesc)
			if err != nil {
				log.Warningf("steps description (`%v`) fetched from db for job %d cannot be serialized: %v", stepDesc, request.JobID, err)
			}

			if string(stepDescDBJSON) != string(stepDescFetchedJSON) {
				log.Warningf("steps retrieved by test fetcher and from database do not match (`%v` != `%v`), test description might have changed", string(stepDescDBJSON), string(stepDescFetchedJSON))
			}

			newStepsDesc.TestName = stepsDescriptors.TestName
			extendedDescriptor.StepsDescriptors = append(extendedDescriptor.StepsDescriptors, newStepsDesc)
		}

		// Serialize job.ExtendedDescriptor
		extendedDescriptorJSON, err := json.Marshal(extendedDescriptor)
		if err != nil {
			return fmt.Errorf("could not serialize extended descriptor for jobID %d (%+v): %w", request.JobID, extendedDescriptor, err)
		}

		insertStatement := "update jobs set extended_descriptor = ?  where job_id = ?"
		log.Debugf("running insert statement: %s, descriptor: %s, jobID: %d", insertStatement, extendedDescriptorJSON, request.JobID)

		insertStart := time.Now()
		_, err = tx.Exec(insertStatement, extendedDescriptorJSON, request.JobID)
		elapsed := time.Since(insertStart)
		log.Debugf("insert statement executed in %.3f ms", ms(elapsed))

		if err != nil {
			return fmt.Errorf("could not store extended descriptor for job_id %d: %w", request.JobID, err)
		}
	}

	elapsed := time.Since(start)
	log.Debugf("completed migrating %d jobs in %.3f ms", len(requests), ms(elapsed))

	return nil
}

// Up implements the forward migration
func (m *DescriptorMigration) Up(tx *sql.Tx) error {

	// Count how many entries we have in jobs table that we need to migrate. Split them into
	// shards of size shardSize for migration. Can't be done online within a single transaction,
	// as there cannot be two active queries on the same connection at the same time
	// (see https://github.com/lib/pq/issues/81)

	count := uint64(0)
	m.log.Debugf("counting the number of jobs to migrate")
	start := time.Now()
	rows, err := tx.Query("select count(*) from jobs")
	if err != nil {
		return fmt.Errorf("could not fetch number of records to migrate: %w", err)
	}
	if !rows.Next() {
		err := "could not fetch number of records to migrate, at least one result from count(*) expected"
		if rows.Err() == nil {
			return fmt.Errorf(err)
		}
		return fmt.Errorf("%s (err: %w)", err, rows.Err())
	}
	if err := rows.Scan(&count); err != nil {
		return fmt.Errorf("could not fetch number of records to migrate: %w", err)
	}
	rows.Close()

	// Create a new plugin registry. This is necessary because some information that need to be
	// associated with the extended_descriptor is not available in the db and can only be looked
	// up via the TestFetcher.
	registry := pluginregistry.NewPluginRegistry()
	plugins.Init(registry, m.log)

	elapsed := time.Since(start)
	m.log.Debugf("total number of jobs to migrate: %d, fetched in %.3f ms", count, ms(elapsed))
	for offset := uint64(0); offset < count; offset += shardSize {
		jobs, err := fetchJobs(tx, shardSize, offset, m.log)
		if err != nil {
			return fmt.Errorf("could not fetch events in range offset %d limit %d: %w", offset, shardSize, err)
		}
		err = migrateJobs(tx, jobs, registry, m.log)
		if err != nil {
			return fmt.Errorf("could not migrate events in range offset %d limit %d: %w", offset, shardSize, err)
		}
	}
	return nil
}

// Down implements the down transition of DescriptorMigration
func (m *DescriptorMigration) Down(tx *sql.Tx) error {
	return nil
}

// NewDescriptorMigration is the factory for DescriptorMigration
func NewDescriptorMigration(log *logrus.Entry) migrate.Migrate {
	return &DescriptorMigration{log: log}
}

// register NewDescriptorMigration at initialization time
func init() {
	migrate.Register(NewDescriptorMigration)
}
