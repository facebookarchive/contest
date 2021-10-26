// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
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
	StoreReport(ctx xcontext.Context, report *job.Report) error
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
	if isStronglyConsistent(ctx) {
		return storage.GetJobRequest(ctx, jobID)
	}
	return storageAsync.GetJobRequest(ctx, jobID)
}

// StoreReport submits a job run or final report to the storage layer
func (jsm JobStorageManager) StoreReport(ctx xcontext.Context, report *job.Report) error {
	return storage.StoreReport(ctx, report)
}

// GetJobReport fetches a job report from the storage layer
func (jsm JobStorageManager) GetJobReport(ctx xcontext.Context, jobID types.JobID) (*job.JobReport, error) {
	if isStronglyConsistent(ctx) {
		return storage.GetJobReport(ctx, jobID)
	}

	return storageAsync.GetJobReport(ctx, jobID)
}

// ListJobs returns list of job IDs matching the query
func (jsm JobStorageManager) ListJobs(ctx xcontext.Context, query *JobQuery) ([]types.JobID, error) {
	if isStronglyConsistent(ctx) {
		return storage.ListJobs(ctx, query)
	}

	return storageAsync.ListJobs(ctx, query)
}

// NewJobStorageManager creates a new JobStorageManager object
func NewJobStorageManager() JobStorageManager {
	return JobStorageManager{}
}
