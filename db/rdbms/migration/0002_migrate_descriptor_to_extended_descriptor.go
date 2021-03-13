// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/tools/migration/rdbms/migrate"

	"github.com/facebookincubator/contest/cmds/plugins"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
)

const shardSize = uint64(5000)

// Contest data structures for migration from v1 to v2. These data structures
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
// In v0001, job.Request object was structured as follows:
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

// Having `TestDescriptors` as a field of request objects creates several issues:
// * It's an abstraction leakage, the description of the steps might not be inlined
//   in the initial request submitted from outside (i.e. might not be a literal embedded
//   in the job descriptor)
// * Storing TestDescriptors directly in the jobs table implies that we track the step
//   descriptions of every test, but not the name of the test themselves, which are part
//   of the test fetcher parameters. This means that to resolve again the full description,
//   which includes names of the tests, we need to go through the test fetcher plugin again.
//   At resume time, we have a dependency on the test fetcher, which in turn depends on
//   fetching the description of the test from the backend, which might not match what was
//   actually submitted at test time.

// Schema v0002 introduces the concept of extended_descriptor, which is defined as follows:
// type ExtendedDescriptor struct {
//		JobDescriptor
//		TestStepsDescriptors []test.TestStepsDescriptors
// }
//
// We remove TestDescriptors from Request objects, and we store that information side-by-side with
// JobDescriptor into an ExtendedDescriptor. We then store this ExtendedDescriptor in the jobs table
// so that all the test information can be re-fetched by reading extended_descriptor field in
// jobs table, without any dependency after submission time on the test fetcher.
type DescriptorMigration struct {
	Context xcontext.Context
}

type dbConn interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

type jobDescriptorPair struct {
	jobID              types.JobID
	extendedDescriptor string
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

// fetchJobs fetches job requests based on limit and offset
func (m *DescriptorMigration) fetchJobs(db dbConn, limit, offset uint64) ([]Request, error) {
	log := m.Context.Logger()

	log.Debugf("fetching shard limit: %d, offset: %d", limit, offset)
	selectStatement := "select job_id, name, requestor, server_id, request_time, descriptor, teststeps from jobs limit ? offset ?"
	log.Debugf("running query: %s", selectStatement)

	start := time.Now()
	rows, err := db.Query(selectStatement, limit, offset)

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

		// `teststeps` column was added relateively late to the `jobs` table, so
		// pre-existing columns have acquired a NULL value. This needs to be taken
		// into account during migration
		testDescriptors := sql.NullString{}

		err := rows.Scan(
			&job.JobID,
			&job.JobName,
			&job.Requestor,
			&job.ServerID,
			&job.RequestTime,
			&job.JobDescriptor,
			&testDescriptors,
		)
		if err != nil {
			return nil, fmt.Errorf("could not scan job request (limit %d, offset %d): %w", limit, offset, err)
		}
		if testDescriptors.Valid {
			job.TestDescriptors = testDescriptors.String
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

func (m *DescriptorMigration) migrateJobs(db dbConn, requests []Request, registry *pluginregistry.PluginRegistry) error {
	log := m.Context.Logger()

	log.Debugf("migrating %d jobs", len(requests))
	start := time.Now()

	var updates []jobDescriptorPair
	for _, request := range requests {

		// Merge JobDescriptor and [][]TestStepDescriptor into a single ExtendedDescriptor.
		// ExtendedDescriptor contains TestStepsDescriptors, whose tpye is declared as follows:
		// type TestStepsDescriptors struct {
		//		TestName    string
		// 		TestSteps   []*TestStepDescriptor
		// }
		//
		// TestStepDescriptor is instead defined as follows:
		// type TestStepDescriptor struct {
		//		Name       string
		//		Label      string
		//		Parameters StepParameters
		// }
		//
		// The previous request.TestDescriptors was actually referring to the JSON
		// representation of the steps, for every test (without any reference to the
		// test name, that would only be part of the global JobDescriptor). This is
		// very ambiguous because to rebuild all the information of a test (for example
		// upon resume, we need to merge information coming from two different places,
		// the steps description in TestDescriptors and the test name in the top level
		// JobDescriptor.
		//
		// So ExtendedDescriptor holds instead to TestStepsDescriptors, which includes
		// test name and test step information.
		//
		// TestDescriptorsfield is removed from request object.

		var jobDesc job.Descriptor
		if err := json.Unmarshal([]byte(request.JobDescriptor), &jobDesc); err != nil {
			return fmt.Errorf("failed to unmarshal job descriptor (%+v): %w", jobDesc, err)
		}

		if len(request.TestDescriptors) == 0 {
			// These TestDescriptors were problably acquired from entries which pre-existed
			// in ConTest db before adding the `teststeps` column. Just skip the migration
			// of these entries.
			log.Debugf("job request with job id %d has null teststeps value, skipping migration", request.JobID)
			continue
		}

		var stepDescs [][]*test.TestStepDescriptor
		if err := json.Unmarshal([]byte(request.TestDescriptors), &stepDescs); err != nil {
			return fmt.Errorf("failed to unmarshal test step descriptors from request object (%+v): %w", request.TestDescriptors, err)
		}

		extendedDescriptor := job.ExtendedDescriptor{Descriptor: jobDesc}
		if len(stepDescs) != len(jobDesc.TestDescriptors) {
			return fmt.Errorf("number of tests described in JobDescriptor does not match steps stored in db")
		}

		for index, stepDesc := range stepDescs {
			newStepsDesc := test.TestStepsDescriptors{}
			// TestName is normally part of TestFetcher parameters, but it's responsibility
			// of the test fetcher to return the name of the Test from the Fetch signature.
			// So, to complete backfill of the data, we initialize directly a TestFetcher
			// and let it retrieve the test name.
			newStepsDesc.TestSteps = append(newStepsDesc.TestSteps, stepDesc...)

			// Look up the original TestDescriptor from JobDescriptor, instantiate
			// TestFetcher accordingly and retrieve the name of the test
			td := jobDesc.TestDescriptors[index]

			tfb, err := registry.NewTestFetcherBundle(m.Context, td)
			if err != nil {
				return fmt.Errorf("could not instantiate test fetcher for jobID %d based on descriptor %+v: %w", request.JobID, td, err)
			}

			name, stepDescFetched, err := tfb.TestFetcher.Fetch(m.Context, tfb.FetchParameters)
			if err != nil {
				return fmt.Errorf("could not retrieve test description from fetcher for jobID %d: %w", request.JobID, err)
			}

			// Check that the serialization of the steps retrieved by the test fetcher matches the steps
			// stored in the DB. If that's not the case, then, just print a warning: the underlying test
			/// might have changed.We go ahead anyway assuming assume the test name is still relevant.
			stepDescFetchedJSON, err := json.Marshal(stepDescFetched)
			if err != nil {
				log.Warnf("steps description (`%v`) fetched by test fetcher for job %d cannot be serialized: %v", stepDescFetched, request.JobID, err)
			}

			stepDescDBJSON, err := json.Marshal(stepDesc)
			if err != nil {
				log.Warnf("steps description (`%v`) fetched from db for job %d cannot be serialized: %v", stepDesc, request.JobID, err)
			}

			if string(stepDescDBJSON) != string(stepDescFetchedJSON) {
				log.Warnf("steps retrieved by test fetcher and from database do not match (`%v` != `%v`), test description might have changed", string(stepDescDBJSON), string(stepDescFetchedJSON))
			}

			newStepsDesc.TestName = name
			extendedDescriptor.TestStepsDescriptors = append(extendedDescriptor.TestStepsDescriptors, newStepsDesc)
		}

		// Serialize job.ExtendedDescriptor
		extendedDescriptorJSON, err := json.Marshal(extendedDescriptor)
		if err != nil {
			return fmt.Errorf("could not serialize extended descriptor for jobID %d (%+v): %w", request.JobID, extendedDescriptor, err)
		}

		updates = append(updates, jobDescriptorPair{jobID: request.JobID, extendedDescriptor: string(extendedDescriptorJSON)})
	}

	if len(updates) == 0 {
		return nil
	}

	var (
		casePlaceholders  []string
		wherePlaceholders []string
	)

	for range updates {
		casePlaceholders = append(casePlaceholders, "when ? then ?")
		wherePlaceholders = append(wherePlaceholders, "?")
	}
	insertStatement := fmt.Sprintf("update jobs set extended_descriptor = case job_id %s end where job_id in (%s)", strings.Join(casePlaceholders, " "), strings.Join(wherePlaceholders, ","))
	log.Debugf("running insert statement with updates: %s, updates: %+v", insertStatement, updates)

	insertStart := time.Now()
	var args []interface{}

	for _, v := range updates {
		args = append(args, v.jobID)
		args = append(args, v.extendedDescriptor)
	}

	for _, v := range updates {
		args = append(args, v.jobID)
	}

	_, err := db.Exec(insertStatement, args...)
	if err != nil {
		var jobIDs []types.JobID
		for _, v := range updates {
			jobIDs = append(jobIDs, v.jobID)
		}
		return fmt.Errorf("could not store extended descriptor (%w) for job ids: %v", err, jobIDs)
	}

	elapsed := time.Since(insertStart)
	log.Debugf("insert statement executed in %.3f ms", ms(elapsed))

	elapsedStart := time.Since(start)
	log.Debugf("completed migrating %d jobs in %.3f ms", len(requests), ms(elapsedStart))

	return nil
}

// Up implements the forward migration
func (m *DescriptorMigration) Up(tx *sql.Tx) error {
	return m.up(tx)
}

// UpNoTx implements the forward migration in a non-transactional manner
func (m *DescriptorMigration) UpNoTx(db *sql.DB) error {
	return m.up(db)
}

// Down implements the down migration of DescriptorMigration
func (m *DescriptorMigration) Down(tx *sql.Tx) error {
	return nil
}

// DownNoTx implements the down migration of DescriptorMigration in a non-transactional manner
func (m *DescriptorMigration) DownNoTx(db *sql.DB) error {
	return nil
}

// up implements the actual migration logic via dbConn interface, which could implement a
// transactional or non-transactional connection, depending on what the caller decided.
func (m *DescriptorMigration) up(db dbConn) error {
	// Count how many entries we have in jobs table that we need to migrate. Split them into
	// shards of size shardSize for migration. Can't be done online within a single transaction,
	// as there cannot be two active queries on the same connection at the same time
	// (see https://github.com/lib/pq/issues/81)

	count := uint64(0)
	ctx := m.Context
	ctx.Logger().Debugf("counting the number of jobs to migrate")
	start := time.Now()
	rows, err := db.Query("select count(*) from jobs")
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
	if err := rows.Close(); err != nil {
		ctx.Logger().Warnf("could not close rows after count(*) query")
	}

	// Create a new plugin registry. This is necessary because some information that need to be
	// associated with the extended_descriptor is not available in the db and can only be looked
	// up via the TestFetcher.
	registry := pluginregistry.NewPluginRegistry(ctx)
	plugins.Init(registry, ctx.Logger())

	elapsed := time.Since(start)
	ctx.Logger().Debugf("total number of jobs to migrate: %d, fetched in %.3f ms", count, ms(elapsed))
	for offset := uint64(0); offset < count; offset += shardSize {
		jobs, err := m.fetchJobs(db, shardSize, offset)
		if err != nil {
			return fmt.Errorf("could not fetch events in range offset %d limit %d: %w", offset, shardSize, err)
		}
		err = m.migrateJobs(db, jobs, registry)
		if err != nil {
			return fmt.Errorf("could not migrate events in range offset %d limit %d: %w", offset, shardSize, err)
		}
		ctx.Logger().Infof("migrated %d/%d", offset, count)
	}
	return nil
}

// NewDescriptorMigration is the factory for DescriptorMigration
func NewDescriptorMigration(ctx xcontext.Context) migrate.Migrate {
	return &DescriptorMigration{
		Context: ctx,
	}
}

// register NewDescriptorMigration at initialization time
func init() {
	migrate.Register(NewDescriptorMigration)
}
