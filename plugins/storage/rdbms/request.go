// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rdbms

import (
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

// StoreJobRequest stores a new job request in the database
func (r *RDBMS) StoreJobRequest(request *job.Request, testDescriptors [][]*test.TestStepDescriptor) (types.JobID, error) {

	var jobID types.JobID

	r.lockTx()
	defer r.unlockTx()

	jsonTestDescs, err := json.Marshal(testDescriptors)
	if err != nil {
		return jobID, fmt.Errorf("failed to marshal test descriptors: %w", err)
	}

	// store job descriptor
	insertStatement := "insert into jobs (name, descriptor, teststeps, requestor, request_time) values (?, ?, ?, ?, ?)"
	result, err := r.db.Exec(insertStatement, request.JobName, request.JobDescriptor, jsonTestDescs, request.Requestor, request.RequestTime)
	if err != nil {
		return jobID, fmt.Errorf("could not store job request in database: %w", err)
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return jobID, fmt.Errorf("could not extract id of last request inserted into db")
	}
	jobID = types.JobID(lastID)
	log.Infof("Got JOB ID %d", jobID)

	return jobID, nil
}

// GetJobRequest retrieves a JobRequest from the database
func (r *RDBMS) GetJobRequest(jobID types.JobID) (*job.Request, [][]*test.TestStepDescriptor, error) {

	r.lockTx()
	defer r.unlockTx()

	selectStatement := "select job_id, name, requestor, request_time, descriptor, teststeps from jobs where job_id = ?"
	log.Debugf("Executing query: %s", selectStatement)
	rows, err := r.db.Query(selectStatement, jobID)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get job request with id %v: %v", jobID, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warningf("could not close rows for job request: %v", err)
		}
	}()

	var (
		req               *job.Request
		testStepDescsJSON []byte
		testStepDescs     [][]*test.TestStepDescriptor
	)
	found := false
	for rows.Next() {
		if req != nil {
			// We have already found a matching request. If we find more than one,
			// then we have a problem
			return nil, nil, fmt.Errorf("multiple requests found with job id %v", jobID)
		}
		found = true
		currRequest := job.Request{}
		err := rows.Scan(
			&currRequest.JobID,
			&currRequest.JobName,
			&currRequest.Requestor,
			&currRequest.RequestTime,
			&currRequest.JobDescriptor,
			&testStepDescsJSON,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("could not get job request with job id %v: %v", jobID, err)
		}
		req = &currRequest
	}
	if !found {
		return nil, nil, fmt.Errorf("no job request found for job ID %d", jobID)
	}
	if err := json.Unmarshal(testStepDescsJSON, &testStepDescs); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal test step descriptors: %w", err)
	}

	if req == nil {
		return nil, nil, fmt.Errorf("could not find request with JobID %d", jobID)
	}
	return req, testStepDescs, nil
}
