// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rdbms

import (
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

// StoreJobRequest stores a new job request in the database
func (r *RDBMS) StoreJobRequest(request *job.Request) (types.JobID, error) {

	var jobID types.JobID

	r.lockTx()
	defer r.unlockTx()

	// serialize the extended descriptor
	extendedDescriptor, err := json.Marshal(request.ExtendedDescriptor)
	if err != nil {
		return jobID, err
	}

	// store job descriptor
	insertStatement := "insert into jobs (name, descriptor, requestor, server_id, request_time) values (?, ?, ?, ?, ?)"
	result, err := r.db.Exec(insertStatement, request.JobName, extendedDescriptor, request.Requestor, request.ServerID, request.RequestTime)
	if err != nil {
		return jobID, fmt.Errorf("could not store job request in database: %w", err)
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

	r.lockTx()
	defer r.unlockTx()

	selectStatement := "select job_id, name, requestor, server_id, request_time, descriptor from jobs where job_id = ?"
	log.Debugf("Executing query: %s", selectStatement)
	rows, err := r.db.Query(selectStatement, jobID)
	if err != nil {
		return nil, fmt.Errorf("could not get job request with id %v: %v", jobID, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warningf("could not close rows for job request: %v", err)
		}
	}()

	var (
		req                    *job.Request
		extendedDescriptorJSON string
	)

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
			&currRequest.ServerID,
			&currRequest.RequestTime,
			&extendedDescriptorJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("could not get job request with job id %v: %v", jobID, err)
		}
		req = &currRequest
	}
	if req == nil {
		return nil, fmt.Errorf("could not find request with JobID %d", jobID)
	}
	extendedDescriptor := job.ExtendedDescriptor{}
	if err := json.Unmarshal([]byte(extendedDescriptorJSON), &extendedDescriptor); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extended job descriptor: %w", err)
	}
	req.ExtendedDescriptor = &extendedDescriptor
	return req, nil
}
