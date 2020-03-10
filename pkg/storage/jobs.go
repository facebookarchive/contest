// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

// JobRequestEmitter implements RequestEmitter interface from the job package
type JobRequestEmitter struct {
}

// JobRequestFetcher implements the RequestRetriever interface from the job package
type JobRequestFetcher struct {
}

// JobRequestEmitterFetcher implements the RequestEmitter and RequestRetriever
// interfaces from the job package
type JobRequestEmitterFetcher struct {
	JobRequestEmitter
	JobRequestFetcher
}

// Emit persists a new job request into storage
func (rc JobRequestEmitter) Emit(request *job.Request, testDescriptors [][]*test.TestStepDescriptor) (types.JobID, error) {
	var jobID types.JobID
	jobID, err := storage.StoreJobRequest(request, testDescriptors)
	if err != nil {
		return jobID, fmt.Errorf("could not store job request: %w", err)
	}
	return jobID, nil
}

// Fetch fetches a Job request from storage based on job id
func (rf JobRequestFetcher) Fetch(jobID types.JobID) (*job.Request, [][]*test.TestStepDescriptor, error) {
	request, testDescriptors, err := storage.GetJobRequest(jobID)
	if err != nil {
		return nil, nil, fmt.Errorf("could not fetch job request: %w", err)
	}
	return request, testDescriptors, nil
}

// NewJobRequestEmitter creates a JobRequestEmitter object
func NewJobRequestEmitter() job.RequestEmitter {
	return JobRequestEmitter{}
}

// NewJobRequestFetcher creates a JobRequestFetcher object
func NewJobRequestFetcher() job.RequestFetcher {
	return JobRequestFetcher{}
}

// NewJobRequestEmitterFetcher creates a JobRequestEmitterFetcher object
func NewJobRequestEmitterFetcher() job.RequestEmitterFetcher {
	return JobRequestEmitterFetcher{
		JobRequestEmitter{},
		JobRequestFetcher{},
	}
}
