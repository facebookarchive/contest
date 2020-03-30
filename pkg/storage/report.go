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

// JobReportEmitter implements ReportEmitter interface from the job package
type JobReportEmitter struct {
}

// JobReportFetcher implements the Fetcher interface from the events package
type JobReportFetcher struct {
}

// JobReportEmitterFetcher implements Emitter and Fetcher interface, using a storage layer
type JobReportEmitterFetcher struct {
	JobReportEmitter
	JobReportFetcher
}

// EmitReport emits the report using the selected storage layer
func (e JobReportEmitter) EmitReport(jobReport *job.JobReport) error {
	if err := storage.StoreJobReport(jobReport); err != nil {
		return fmt.Errorf("could not persist job report: %v", err)
	}
	return nil
}

// FetchReport retrieves job report objects based on JobID
func (ev JobReportFetcher) FetchReport(jobID types.JobID) (*job.JobReport, error) {
	report, err := storage.GetJobReport(jobID)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// NewJobReportEmitter creates a new emitter object for Job reports
func NewJobReportEmitter() job.ReportEmitter {
	return JobReportEmitter{}
}

// NewJobReportFetcher creates a new fetcher object for Job reports
func NewJobReportFetcher() job.ReportFetcher {
	return JobReportFetcher{}
}

// NewJobReportEmitterFetcher creates a new EmitterFetcher object for Job reports
func NewJobReportEmitterFetcher() job.ReportEmitterFetcher {
	return JobReportEmitterFetcher{}
}
