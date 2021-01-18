// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rdbms

import (
	"fmt"
	"strings"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
)

func (r *RDBMS) ListJobs(query *storage.JobQuery) ([]types.JobID, error) {
	res := []types.JobID{}

	// Quoting SQL strings is hard. https://github.com/golang/go/issues/18478
	// For now, just disallow anything that is not [a-zA-Z0-9_-]
	if err := job.CheckTags(query.Tags); err != nil {
		return nil, err
	}

	// Job state is updated by framework events, ensure there aren't any pending.
	r.frameworkEventsLock.Lock()
	err := r.flushFrameworkEvents()
	r.frameworkEventsLock.Unlock()
	if err != nil {
		return nil, fmt.Errorf("could not flush events before reading events: %v", err)
	}

	// Construct the query.
	parts := []string{"SELECT jobs.job_id FROM jobs"}
	qargs := []interface{}{}
	// Tag filtering uses joins, 1 per tag.
	for i := range query.Tags {
		parts = append(parts, fmt.Sprintf(`INNER JOIN job_tags jt%d ON jobs.job_id = jt%d.job_id`, i, i))
	}
	var conds []string
	if len(query.States) > 0 {
		stst := make([]string, len(query.States))
		for i, st := range query.States {
			stst[i] = "?"
			qargs = append(qargs, st)
		}
		conds = append(conds, fmt.Sprintf("jobs.state IN (%s)", strings.Join(stst, ", ")))
	}
	// Now the corresponding conditions.
	for i, tag := range query.Tags {
		conds = append(conds, fmt.Sprintf("jt%d.tag = ?", i))
		qargs = append(qargs, tag)
	}
	if len(conds) > 0 {
		parts = append(parts, "WHERE", strings.Join(conds, " AND "))
	}
	/* Examples of the resulting queries:
	SELECT jobs.job_id FROM jobs
	SELECT jobs.job_id FROM jobs WHERE jobs.state IN (0)
	SELECT jobs.job_id FROM jobs WHERE jobs.state IN (2, 3)
	SELECT jobs.job_id FROM jobs INNER JOIN job_tags jt0 ON jobs.job_id = jt0.job_id WHERE jt0.tag = "tests"
	SELECT jobs.job_id FROM jobs INNER JOIN job_tags jt0 ON jobs.job_id = jt0.job_id INNER JOIN job_tags jt1 ON jobs.job_id = jt1.job_id WHERE jobs.state IN (2, 3, 4) AND jt0.tag = "tests" AND jt1.tag = "foo"
	*/
	parts = append(parts, "ORDER BY jobs.job_id")
	stmt := strings.Join(parts, " ")

	rows, err := r.db.Query(stmt, qargs...)
	if err != nil {
		return nil, fmt.Errorf("could not list jobs in states (sql: %q): %w", stmt, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		var jobID types.JobID
		if err := rows.Scan(&jobID); err != nil {
			return nil, fmt.Errorf("could not list jobs in states (sql: %q): %w", stmt, err)
		}
		res = append(res, jobID)
	}

	return res, nil
}
