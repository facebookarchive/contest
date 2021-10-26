// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rdbms

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// StoreReport persists a run or final report in the internal storage.
func (r *RDBMS) StoreReport(_ xcontext.Context, report *job.Report) error {

	reportJSON, err := report.ToJSON()
	if err != nil {
		return fmt.Errorf("could not serialize final report for job %v: %v", report.JobID, err)
	}

	r.lockTx()
	defer r.unlockTx()

	if report.RunID > 0 {
		if _, err := r.db.Exec(
			"insert into run_reports (job_id, run_id, reporter_name, success, report_time, data) values (?, ?, ?, ?, ?, ?)",
			report.JobID, report.RunID, report.ReporterName, report.Success, report.ReportTime, reportJSON); err != nil {
			return fmt.Errorf("could not store run report for job %v: %v", report.JobID, err)
		}
	} else {
		if _, err := r.db.Exec(
			"insert into final_reports (job_id, reporter_name, success, report_time, data) values (?, ?, ?, ?, ?)",
			report.JobID, report.ReporterName, report.Success, report.ReportTime, reportJSON); err != nil {
			return fmt.Errorf("could not store final report for job %v: %v", report.JobID, err)
		}
	}
	return nil
}

// GetJobReport retrieves a JobReport from the database
func (r *RDBMS) GetJobReport(ctx xcontext.Context, jobID types.JobID) (*job.JobReport, error) {

	var (
		runReports        [][]*job.Report
		currentRunReports []*job.Report
		finalReports      []*job.Report
	)

	r.lockTx()
	defer r.unlockTx()

	// get run reports. Don't change the order by asc, because
	// the code below assumes sorted results by ascending run number.
	selectStatement := "select success, report_time, reporter_name, run_id, data from run_reports where job_id = ? order by run_id asc, reporter_name asc"
	ctx.Debugf("Executing query: %s", selectStatement)
	rows, err := r.db.Query(selectStatement, jobID)
	if err != nil {
		return nil, fmt.Errorf("could not get run report for job %v: %v", jobID, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			ctx.Warnf("failed to close rows from query statement: %v", err)
		}
	}()
	var lastRunID, currentRunID uint
	for rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("could not fetch run report for job %d: %v", jobID, err)
		}
		var (
			report job.Report
			data   string
		)
		err = rows.Scan(
			&report.Success,
			&report.ReportTime,
			&report.ReporterName,
			&currentRunID,
			&data,
		)
		// Fetch fetches a Job request from storage based on job id
		if err != nil {
			return nil, fmt.Errorf("failed to scan row while fetching run report for job %d: %v", jobID, err)
		}
		if err := json.Unmarshal([]byte(data), &report.Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal run report JSON data: %v", err)
		}
		// rows are sorted by ascending run_id, so if we find a
		// non-monotonic run_id or a gap, we return an error.
		// This works as long as we can assume ascending sorting, so don't
		// change it, or at least change both.
		if currentRunID == 0 {
			return nil, errors.New("invalid run_id in database, cannot be zero")
		}
		if currentRunID < lastRunID || currentRunID > lastRunID+1 {
			return nil, fmt.Errorf("invalid run_id retrieved from database: either it is not ordered, or there is a gap in run numbers in the database for job %d. Current run number: %d, last run number: %d",
				jobID, currentRunID, lastRunID,
			)
		}
		if currentRunID != lastRunID {
			// this is the next run number
			if lastRunID > 0 {
				runReports = append(runReports, currentRunReports)
				currentRunReports = make([]*job.Report, 0)
			}
			lastRunID = currentRunID
		}
		// These struct fields were added later, populate them from columns.
		report.JobID = jobID
		report.RunID = types.RunID(currentRunID)
		currentRunReports = append(currentRunReports, &report)
	}
	if len(currentRunReports) > 0 {
		runReports = append(runReports, currentRunReports)
	}

	// get final reports
	selectStatement = "select success, report_time, reporter_name, data from final_reports where job_id = ? order by reporter_name asc"
	ctx.Debugf("Executing query: %s", selectStatement)
	rows2, err := r.db.Query(selectStatement, jobID)
	if err != nil {
		return nil, fmt.Errorf("could not get final report for job %v: %v", jobID, err)
	}
	defer func() {
		if err := rows2.Close(); err != nil {
			ctx.Warnf("failed to close rows2 from query statement: %v", err)
		}
	}()
	for rows2.Next() {
		if err := rows2.Err(); err != nil {
			return nil, fmt.Errorf("could not fetch final report for job %d: %v", jobID, err)
		}
		var (
			report job.Report
			data   string
		)
		err = rows2.Scan(
			&report.Success,
			&report.ReportTime,
			&report.ReporterName,
			&data,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row while fetching final report for job %d: %v", jobID, err)
		}
		if err := json.Unmarshal([]byte(data), &report.Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal final report JSON data: %v", err)
		}
		// These struct fields were added later, populate them from columns.
		report.JobID = jobID
		report.RunID = 0
		finalReports = append(finalReports, &report)
	}
	return &job.JobReport{
		JobID:        jobID,
		RunReports:   runReports,
		FinalReports: finalReports,
	}, nil
}
