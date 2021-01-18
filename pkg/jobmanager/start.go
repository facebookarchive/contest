// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/job"
)

func (jm *JobManager) start(ev *api.Event) *api.EventResponse {
	msg := ev.Msg.(api.EventStartMsg)
	j, err := NewJob(jm.pluginRegistry, msg.JobDescriptor)
	if err != nil {
		return &api.EventResponse{Err: err}
	}
	// The job descriptor has been validated correctly, now use the JobRequestEmitter
	// interface to obtain a JobRequest object with a valid id
	request := job.Request{
		JobName:         j.Name,
		Requestor:       string(ev.Msg.Requestor()),
		ServerID:        ev.ServerID,
		RequestTime:     time.Now(),
		JobDescriptor:   msg.JobDescriptor,
		TestDescriptors: j.TestDescriptors,
	}
	jobID, err := jm.jobStorageManager.StoreJobRequest(&request)
	if err != nil {
		return &api.EventResponse{
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not create job request: %v", err)}
	}
	j.ID = jobID
	if err := jm.emitEvent(j.ID, job.EventJobStarted); err != nil {
		return &api.EventResponse{
			Requestor: ev.Msg.Requestor(),
			Err:       err,
		}
	}

	jm.jobsWg.Add(1)
	go func() {
		defer jm.jobsWg.Done()

		jm.jobsMu.Lock()
		jm.jobs[j.ID] = j
		jm.jobsMu.Unlock()

		start := time.Now()
		runReports, finalReports, err := jm.jobRunner.Run(j)
		duration := time.Since(start)
		log.Debugf("job %d terminated", j.ID)
		// If the Job was cancelled/paused, the error returned by JobRunner indicates whether
		// the cancellation/pausing has been successful or failed
		switch {
		case j.IsCancelled():
			if err != nil {
				errCancellation := fmt.Errorf("Job %+v failed cancellation: %v", j, err)
				log.Error(errCancellation)
				_ = jm.emitErrEvent(jobID, job.EventJobCancellationFailed, errCancellation)
			} else {
				_ = jm.emitEvent(jobID, job.EventJobCancelled)
			}
			return
		case j.IsPaused():
			if err != nil {
				errPausing := fmt.Errorf("Job %+v failed pausing: %v", j, err)
				log.Error(errPausing)
				_ = jm.emitErrEvent(jobID, job.EventJobPauseFailed, errPausing)
			} else {
				_ = jm.emitEvent(jobID, job.EventJobPaused)
			}
			return
		}

		// store job report before emitting the job status event, to avoid a
		// race condition when waiting on a job status where the event is marked
		// as completed but no report exists.
		jobReport := job.JobReport{
			JobID:        j.ID,
			RunReports:   runReports,
			FinalReports: finalReports,
		}
		if storageErr := jm.jobStorageManager.StoreJobReport(&jobReport); storageErr != nil {
			log.Warningf("Could not emit job report: %v", storageErr)
		}
		// at this point it is safe to emit the job status event. Note: this is
		// checking `err` from the `jm.jobRunner.Run()` call above.
		if err != nil {
			errMsg := fmt.Sprintf("Job %+v failed after %s : %v", j, duration, err)
			log.Errorf(errMsg)
			_ = jm.emitErrEvent(jobID, job.EventJobFailed, err)
		} else {
			// If the JobManager doesn't return any error, the outcome of the Job
			// might have been any of the following:
			// * Job completed successfully
			// * Job was cancelled
			// * Job was paused
			var eventToEmit event.Name
			switch {
			case j.IsCancelled():
				log.Infof("Job %+v completed cancellation", j)
				eventToEmit = job.EventJobCancelled
			case j.IsPaused():
				log.Infof("Job %+v completed pausing", j)
				eventToEmit = job.EventJobPaused
			default:
				log.Infof("Job %+v completed after %s", j, duration)
				eventToEmit = job.EventJobCompleted
			}
			log.Debugf("emitting: %v", eventToEmit)
			err = jm.emitEvent(jobID, eventToEmit)
			if err != nil {
				log.Warningf("event emission failed: %v", err)
			}
		}
	}()

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
