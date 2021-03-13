// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// JobStorage defines the interface that implements persistence for job
// related information
type JobStorage interface {
	// Job request interface
	StoreJobRequest(ctx xcontext.Context, request *job.Request) (types.JobID, error)
	GetJobRequest(ctx xcontext.Context, jobID types.JobID) (*job.Request, error)

	// Job report interface
	StoreJobReport(ctx xcontext.Context, report *job.JobReport) error
	GetJobReport(ctx xcontext.Context, jobID types.JobID) (*job.JobReport, error)

	// Job enumeration interface
	ListJobs(ctx xcontext.Context, query *JobQuery) ([]types.JobID, error)
}

// JobStorageManager implements JobStorage interface
type JobStorageManager struct {
}

// StoreJobRequest submits a job request to the storage layer
func (jsm JobStorageManager) StoreJobRequest(ctx xcontext.Context, request *job.Request) (types.JobID, error) {
	return storage.StoreJobRequest(ctx, request)
}

// GetJobRequest fetches a job request from the storage layer
func (jsm JobStorageManager) GetJobRequest(ctx xcontext.Context, jobID types.JobID) (*job.Request, error) {
	return storage.GetJobRequest(ctx, jobID)
}

// GetJobRequest fetches a job request from the read-only storage layer
func (jsm JobStorageManager) GetJobRequestAsync(ctx xcontext.Context, jobID types.JobID) (*job.Request, error) {
	request, err := storageAsync.GetJobRequest(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("could not fetch job request: %v", err)
	}
	return request, nil
}

// StoreJobReport submits a job report to the storage layer
func (jsm JobStorageManager) StoreJobReport(ctx xcontext.Context, report *job.JobReport) error {
	return storage.StoreJobReport(ctx, report)
}

// GetJobReport fetches a job report from the storage layer
func (jsm JobStorageManager) GetJobReport(ctx xcontext.Context, jobID types.JobID) (*job.JobReport, error) {
	return storage.GetJobReport(ctx, jobID)
}

// ListJobs returns list of job IDs matching the query
func (jsm JobStorageManager) ListJobs(ctx xcontext.Context, query *JobQuery) ([]types.JobID, error) {
	return storage.ListJobs(ctx, query)
}

// GetJobReportAsync fetches a job report from the read-only storage layer
func (jsm JobStorageManager) GetJobReportAsync(ctx xcontext.Context, jobID types.JobID) (*job.JobReport, error) {
	report, err := storageAsync.GetJobReport(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// NewJobStorageManager creates a new JobStorageManager object
func NewJobStorageManager() JobStorageManager {
	return JobStorageManager{}
}
