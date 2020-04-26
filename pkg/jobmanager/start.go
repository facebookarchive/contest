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
	if err := jm.emitEvent(j.ID, EventJobStarted); err != nil {
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
		// If the Job was cancelled, the error returned by JobRunner indicates whether
		// the cancellatioon has been successful or failed
		if j.IsCancelled() {
			if err != nil {
				errCancellation := fmt.Errorf("Job %+v failed cancellation: %v", j, err)
				log.Error(errCancellation)
				_ = jm.emitErrEvent(jobID, EventJobCancellationFailed, errCancellation)
			} else {
				_ = jm.emitEvent(jobID, EventJobCancelled)
			}
			return
		}

		if err != nil {
			errMsg := fmt.Sprintf("Job %+v failed after %s : %v", j, duration, err)
			log.Errorf(errMsg)
			_ = jm.emitErrEvent(jobID, EventJobFailed, err)
		} else {
			// If the JobManager doesn't return any error, the outcome of the Job
			// might have been any of the following:
			// * Job completed successfully
			// * Job was cancelled
			var eventToEmit event.Name
			if j.IsCancelled() {
				log.Infof("Job %+v completed cancellation", j)
				eventToEmit = EventJobCancelled
			} else {
				log.Infof("Job %+v completed after %s", j, duration)
				eventToEmit = EventJobCompleted
			}
			_ = jm.emitEvent(jobID, eventToEmit)
		}
		jobReport := job.JobReport{
			JobID:        j.ID,
			RunReports:   runReports,
			FinalReports: finalReports,
		}
		err = jm.jobStorageManager.StoreJobReport(&jobReport)
		if err != nil {
			log.Warningf("Could not emit job report: %v", err)
		}
	}()

	return &api.EventResponse{
		JobID:     j.ID,
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
		Status: &job.Status{
			Name:      j.Name,
			State:     string(EventJobStarted),
			StartTime: time.Now(),
		},
	}
}
