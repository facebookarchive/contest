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

// StoreJobReport ...
func (r *RDBMS) StoreJobReport(report *job.Report) error {
	if err := r.init(); err != nil {
		return fmt.Errorf("could not initialize database: %v", err)
	}

	insertStatement := "insert into reports (job_id, success, report_time, job_report) values (?, ?, ?, ?)"
	jobReportJSON, err := report.JobReportJSON()
	if err != nil {
		return fmt.Errorf("could not serialize job report for job %v: %v", report.JobID, err)
	}
	if _, err := r.db.Exec(insertStatement, report.JobID, report.Success, report.ReportTime, jobReportJSON); err != nil {
		return fmt.Errorf("could not store job report for job %v: %v", report.JobID, err)
	}
	return nil
}

// GetJobReport retrieves a JobReport from the database
func (r *RDBMS) GetJobReport(jobID types.JobID) (*job.Report, error) {
	if err := r.init(); err != nil {
		return nil, fmt.Errorf("could not initialize database: %v", err)
	}

	selectStatement := "select job_id, success, report_time, job_report from reports where job_id = ?"
	log.Debugf("Executing query: %s", selectStatement)
	rows, err := r.db.Query(selectStatement, jobID)
	if err != nil {
		return nil, fmt.Errorf("could not get job report for job %v: %v", jobID, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warningf("failed to close rows from query statement: %v", err)
		}
	}()

	var jobReportJSON json.RawMessage

	report := job.Report{}
	report.JobReport = jobReportJSON

	rows.Next()
	rr := ""
	err = rows.Scan(
		&report.JobID,
		&report.Success,
		&report.ReportTime,
		&rr,
	)
	if err != nil {
		return nil, fmt.Errorf("could not get job report for job %v: %v", jobID, err)
	}
	err = json.Unmarshal([]byte(rr), &report.JobReport)
	if err != nil {
		return nil, fmt.Errorf("could not get job report for job %v: %v", jobID, err)
	}
	return &report, nil
}
