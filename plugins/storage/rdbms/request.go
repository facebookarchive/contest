// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rdbms

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

// StoreJobRequest stores a new job request in the database
func (r *RDBMS) StoreJobRequest(request *job.Request) (types.JobID, error) {

	var jobID types.JobID

	if err := r.init(); err != nil {
		return jobID, fmt.Errorf("could not initialize database: %v", err)
	}
	insertStatement := "insert into jobs (name, descriptor, requestor, request_time) values (?, ?, ?, ?)"
	result, err := r.db.Exec(insertStatement, request.JobName, request.JobDescriptor, request.Requestor, request.RequestTime)
	if err != nil {
		return jobID, fmt.Errorf("could not store job request in database: %v", err)
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return jobID, fmt.Errorf("could not extract id of last request inserted into db")
	}
	jobID = types.JobID(lastID)
	return jobID, nil
}

// GetJobRequest retrieves a JobRequest from the database
func (r *RDBMS) GetJobRequest(jobID types.JobID) (*job.Request, error) {

	if err := r.init(); err != nil {
		return nil, fmt.Errorf("could not initialize database: %v", err)
	}

	selectStatement := "select job_id, name, requestor, request_time, descriptor from jobs where job_id = ?"
	log.Debugf("Executing query: %s", selectStatement)
	rows, err := r.db.Query(selectStatement, jobID)
	if err != nil {
		return nil, fmt.Errorf("could not get job request with id %v: %v", jobID, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warningf("failed to close rows from query statement: %v", err)
		}
	}()

	var req *job.Request
	for rows.Next() {
		if req != nil {
			// We have already found a matching request. If we find more than one,
			// then we have a problem
			return nil, fmt.Errorf("multiple requests found with job id %v", jobID)
		}
		currRequest := job.Request{}
		err := rows.Scan(
			&currRequest.JobID,
			&currRequest.JobName,
			&currRequest.Requestor,
			&currRequest.RequestTime,
			&currRequest.JobDescriptor,
		)
		if err != nil {
			return nil, fmt.Errorf("could not get job request with job id %v: %v", jobID, err)
		}
		req = &currRequest
	}

	if req == nil {
		return nil, fmt.Errorf("could not find request with JobID %d", jobID)
	}
	return req, nil
}
