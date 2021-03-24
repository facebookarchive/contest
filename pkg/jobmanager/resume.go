// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

func (jm *JobManager) resumeJobs(ctx xcontext.Context, serverID string) error {
	queryFields := []storage.JobQueryField{
		storage.QueryJobServerID(serverID),
		storage.QueryJobStates(job.JobStatePaused),
	}
	if jm.config.instanceTag != "" {
		queryFields = append(queryFields, storage.QueryJobTags(jm.config.instanceTag))
	}
	q, err := storage.BuildJobQuery(queryFields...)
	if err != nil {
		return fmt.Errorf("failed to build job query: %w", err)
	}
	pausedJobs, err := jm.jsm.ListJobs(ctx, q)
	if err != nil {
		return fmt.Errorf("failed to list paused jobs: %w", err)
	}
	ctx.Infof("Found %d paused jobs for %s/%s", len(pausedJobs), jm.config.instanceTag, serverID)
	for _, jobID := range pausedJobs {
		if err := jm.resumeJob(ctx, jobID); err != nil {
			ctx.Errorf("failed to resume job %d: %v, failing it", jobID, err)
			if err = jm.emitErrEvent(ctx, jobID, job.EventJobFailed, fmt.Errorf("failed to resume job %d: %w", jobID, err)); err != nil {
				ctx.Warnf("Failed to emit event for %d: %v", jobID, err)
			}
		}
	}
	return nil
}

func (jm *JobManager) resumeJob(ctx xcontext.Context, jobID types.JobID) error {
	ctx.Debugf("attempting to resume job %d", jobID)
	results, err := jm.frameworkEvManager.Fetch(
		ctx,
		frameworkevent.QueryJobID(jobID),
		frameworkevent.QueryEventName(job.EventJobPaused),
	)
	if err != nil {
		return fmt.Errorf("failed to query resume state for job %d: %w", jobID, err)
	}
	if len(results) == 0 {
		return fmt.Errorf("no resume state found for job %d", jobID)
	}
	// Sort by EmitTime in descending order.
	sort.Slice(results, func(i, j int) bool { return results[i].EmitTime.After(results[j].EmitTime) })
	var resumeState job.PauseEventPayload
	if results[0].Payload == nil {
		return fmt.Errorf("invald resume state for job %d: %+v", jobID, results[0])
	}
	if err := json.Unmarshal(*results[0].Payload, &resumeState); err != nil {
		return fmt.Errorf("invald resume state for job %d: %w", jobID, err)
	}
	if resumeState.Version != job.CurrentPauseEventPayloadVersion {
		return fmt.Errorf("incompatible resume state version (want %d, got %d)",
			job.CurrentPauseEventPayloadVersion, resumeState.Version)
	}
	req, err := jm.jsm.GetJobRequest(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to retrieve job descriptor for %d: %w", jobID, err)
	}
	j, err := NewJobFromExtendedDescriptor(ctx, jm.pluginRegistry, req.ExtendedDescriptor)
	if err != nil {
		return fmt.Errorf("failed to create job %d: %w", jobID, err)
	}
	j.ID = jobID
	ctx.Debugf("running resumed job %d", j.ID)
	jm.startJob(ctx, j, &resumeState)
	return nil
}
