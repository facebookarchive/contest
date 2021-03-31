// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

func (jm *JobManager) start(ev *api.Event) *api.EventResponse {
	msg := ev.Msg.(api.EventStartMsg)

	var jd job.Descriptor
	if err := json.Unmarshal([]byte(msg.JobDescriptor), &jd); err != nil {
		return &api.EventResponse{Err: err}
	}
	if err := job.CheckTags(jd.Tags, false /* allowInternal */); err != nil {
		return &api.EventResponse{Err: err}
	}
	// Add instance tag, if specified.
	if jm.config.instanceTag != "" {
		jd.Tags = job.AddTags(jd.Tags, jm.config.instanceTag)
	}
	j, err := NewJobFromDescriptor(ev.Context, jm.pluginRegistry, &jd)
	if err != nil {
		return &api.EventResponse{Err: err}
	}
	jdJSON, err := json.MarshalIndent(&jd, "", "    ")
	if err != nil {
		return &api.EventResponse{Err: err}
	}

	// The job descriptor has been validated correctly, now use the JobRequestEmitter
	// interface to obtain a JobRequest object with a valid id
	request := job.Request{
		JobName:            j.Name,
		JobDescriptor:      string(jdJSON),
		ExtendedDescriptor: j.ExtendedDescriptor,
		Requestor:          string(ev.Msg.Requestor()),
		ServerID:           ev.ServerID,
		RequestTime:        time.Now(),
	}
	jobID, err := jm.jsm.StoreJobRequest(ev.Context, &request)
	if err != nil {
		return &api.EventResponse{
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not create job request: %v", err)}
	}

	j.ID = jobID

	jm.startJob(ev.Context, j, nil)

	return &api.EventResponse{
		JobID:     j.ID,
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
		Status: &job.Status{
			Name:      j.Name,
			State:     string(job.EventJobStarted),
			StartTime: time.Now(),
		},
	}
}

func (jm *JobManager) startJob(ctx xcontext.Context, j *job.Job, resumeState *job.PauseEventPayload) {
	jm.jobsMu.Lock()
	defer jm.jobsMu.Unlock()
	jobCtx, jobCancel := xcontext.WithCancel(ctx)
	jobCtx, jobPause := xcontext.WithNotify(jobCtx, xcontext.ErrPaused)
	jm.jobs[j.ID] = &jobInfo{job: j, pause: jobPause, cancel: jobCancel}
	go jm.runJob(jobCtx, j, resumeState)
}

func (jm *JobManager) runJob(ctx xcontext.Context, j *job.Job, resumeState *job.PauseEventPayload) {
	defer func() {
		jm.jobsMu.Lock()
		delete(jm.jobs, j.ID)
		jm.jobsMu.Unlock()
	}()

	ctx = ctx.WithField("job_id", j.ID)

	if err := jm.emitEvent(ctx, j.ID, job.EventJobStarted); err != nil {
		ctx.Errorf("failed to emit event: %v", err)
		return
	}

	start := time.Now()
	runReports, finalReports, resumeState, err := jm.jobRunner.Run(ctx, j, resumeState)
	duration := time.Since(start)
	ctx.Debugf("Job %d: runner finished, err %v", j.ID, err)
	switch err {
	case xcontext.ErrCanceled:
		_ = jm.emitEvent(ctx, j.ID, job.EventJobCancelled)
		return
	case xcontext.ErrPaused:
		if err := jm.emitEventPayload(ctx, j.ID, job.EventJobPaused, resumeState); err != nil {
			_ = jm.emitErrEvent(ctx, j.ID, job.EventJobPauseFailed, fmt.Errorf("Job %+v failed pausing: %v", j, err))
		} else {
			ctx.Infof("Successfully paused job %d (run %d, %d targets)", j.ID, resumeState.RunID, len(resumeState.Targets))
			ctx.Debugf("Job %d pause state: %+v", j.ID, resumeState)
		}
		return
	}
	select {
	case <-ctx.Until(xcontext.ErrPaused):
		// We were asked to pause but failed to do so.
		pauseErr := fmt.Errorf("Job %+v failed pausing: %v", j, err)
		ctx.Errorf("%v", pauseErr)
		_ = jm.emitErrEvent(ctx, j.ID, job.EventJobPauseFailed, pauseErr)
		return
	default:
	}
	ctx.Infof("Job %d finished", j.ID)

	// store job report before emitting the job status event, to avoid a
	// race condition when waiting on a job status where the event is marked
	// as completed but no report exists.
	jobReport := job.JobReport{
		JobID:        j.ID,
		RunReports:   runReports,
		FinalReports: finalReports,
	}
	if storageErr := jm.jsm.StoreJobReport(ctx, &jobReport); storageErr != nil {
		ctx.Warnf("Could not emit job report: %v", storageErr)
	}
	// at this point it is safe to emit the job status event. Note: this is
	// checking `err` from the `jm.jobRunner.Run()` call above.
	if err != nil {
		_ = jm.emitErrEvent(ctx, j.ID, job.EventJobFailed, fmt.Errorf("Job %d failed after %s: %w", j.ID, duration, err))
	} else {
		ctx.Infof("Job %+v completed after %s", j, duration)
		err = jm.emitEvent(ctx, j.ID, job.EventJobCompleted)
		if err != nil {
			ctx.Warnf("event emission failed: %v", err)
		}
	}
}
