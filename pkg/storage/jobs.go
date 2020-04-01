// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"errors"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

// JobInfoFetcher implements job.JobInfoFetcher to retrieve job information from
// the database.
type JobInfoFetcher struct{}

// FetchJob returns a job object after rebuilding it from the database.
// If the job doesn't exist or cannot be rebuilt, an error is returned.
func (jf JobInfoFetcher) FetchJob(jobID types.JobID) (*job.Job, error) {
	return nil, errors.New("not implemented yet")
}

// FetchJobs works like FetchJob, but it operates on multiple job IDs. If a job
// doesn't exist or cannot be rebuilt, an error is returned.
func (jf JobInfoFetcher) FetchJobs(jobIDs []types.JobID) ([]*job.Job, error) {
	return nil, errors.New("not implemented yet")
}

// FetchJobIDsByServerID returns all the job IDs belonging to the provided server ID.
func (jf JobInfoFetcher) FetchJobIDsByServerID(serverID string) ([]types.JobID, error) {
	return nil, errors.New("not implemented yet")
}

// NewJobInfoFetcher creates a JobRequestEmitterFetcher object
func NewJobInfoFetcher() job.InfoFetcher {
	return JobInfoFetcher{}
}
