// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

// JobStorageManager implements JobStorage interface
type JobStorageManager struct {
}

// StoreJobRequest submits a job request to the storage layer
func (jsm JobStorageManager) StoreJobRequest(request *job.Request) (types.JobID, error) {
	var jobID types.JobID
	jobID, err := storage.StoreJobRequest(request)
	if err != nil {
		return jobID, fmt.Errorf("could not store job request: %v", err)
	}
	return jobID, nil
}

// GetJobRequest fetches a job request from the storage layer
func (jsm JobStorageManager) GetJobRequest(jobID types.JobID) (*job.Request, error) {
	request, err := storage.GetJobRequest(jobID)
	if err != nil {
		return nil, fmt.Errorf("could not fetch job request: %v", err)
	}
	return request, nil
}

// GetJobRequest fetches a job request from the read-only storage layer
func (jsm JobStorageManager) GetJobRequestAsync(jobID types.JobID) (*job.Request, error) {
	request, err := storageAsync.GetJobRequest(jobID)
	if err != nil {
		return nil, fmt.Errorf("could not fetch job request: %v", err)
	}
	return request, nil
}

// StoreJobReport submits a job report to the storage layer
func (jsm JobStorageManager) StoreJobReport(report *job.JobReport) error {
	if err := storage.StoreJobReport(report); err != nil {
		return fmt.Errorf("could not persist job report: %v", err)
	}
	return nil
}

// GetJobReport fetches a job report from the storage layer
func (jsm JobStorageManager) GetJobReport(jobID types.JobID) (*job.JobReport, error) {
	report, err := storage.GetJobReport(jobID)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// GetJobReportAsync fetches a job report from the read-only storage layer
func (jsm JobStorageManager) GetJobReportAsync(jobID types.JobID) (*job.JobReport, error) {
	report, err := storageAsync.GetJobReport(jobID)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// NewJobStorageManager creates a new JobStorageManager object
func NewJobStorageManager() JobStorageManager {
	return JobStorageManager{}
}
