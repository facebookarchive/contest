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
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
)

func (jm *JobManager) status(ev *api.Event) *api.EventResponse {
	msg := ev.Msg.(api.EventStatusMsg)
	jobID := msg.JobID
	evResp := api.EventResponse{
		JobID:     jobID,
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
	}

	// Fetch all the events associated to changes of state of the Job
	jobEvents, err := jm.frameworkEvManager.Fetch(
		frameworkevent.QueryJobID(jobID),
		frameworkevent.QueryEventNames(JobStateEvents),
	)
	if err != nil {
		evResp.Err = fmt.Errorf("could not fetch events associated to job state: %v", err)
		return &evResp
	}
	req, err := jm.jobStorageManager.GetJobRequest(jobID)
	if err != nil {
		evResp.Err = fmt.Errorf("failed to fetch request for job ID %d: %w", jobID, err)
		return &evResp
	}
	currentJob, err := NewJobFromRequest(jm.pluginRegistry, req)
	if err != nil {
		evResp.Err = fmt.Errorf("failed to build job object from job request: %w", err)
		return &evResp
	}

	// Lookup job starting time and job termination time based on the events emitted
	var (
		startTime time.Time
		endTime   *time.Time
	)

	completionEvents := make(map[event.Name]bool)
	for _, eventName := range JobCompletionEvents {
		completionEvents[eventName] = true
	}

	for _, ev := range jobEvents {
		if ev.EventName == EventJobStarted {
			startTime = ev.EmitTime
		} else if _, ok := completionEvents[ev.EventName]; ok {
			// A completion event has been seen for this Job. Only one completion event can be associated to the job
			if endTime != nil && !endTime.IsZero() {
				log.Warningf("Job %d is associated to multiple completion events", jobID)
			}
			endTime = &ev.EmitTime
		}
	}

	state := "Unknown"
	var stateErrMsg string
	if len(jobEvents) > 0 {
		je := jobEvents[len(jobEvents)-1]
		state = string(je.EventName)
		if je.EventName == EventJobFailed {
			// if there was a framework failure, retrieve the failure event and
			// the associated error message, so it can be exposed in the status.
			if je.Payload == nil {
				stateErrMsg = "internal error: EventJobFailed's payload is nil"
			} else {
				var ep ErrorEventPayload
				if err := json.Unmarshal(*je.Payload, &ep); err != nil {
					stateErrMsg = fmt.Sprintf("internal error: EventJobFailed's payload cannot be unmarshalled. Raw payload: %s, Error: %v", *je.Payload, err)
				} else {
					stateErrMsg = ep.Err
				}
			}
		}
	}

	report, err := jm.jobStorageManager.GetJobReport(jobID)
	if err != nil {
		evResp.Err = fmt.Errorf("could not fetch job report: %v", err)
		return &evResp
	}

	jobStatus := job.Status{
		Name:        currentJob.Name,
		StartTime:   startTime,
		EndTime:     endTime,
		State:       state,
		StateErrMsg: stateErrMsg,
		JobReport:   report,
	}

	// Fetch the ID of the last run that was started
	runID, err := jm.jobRunner.GetCurrentRun(jobID)
	if err != nil {
		evResp.Err = fmt.Errorf("could not determine the current run id being executed: %v", err)
		return &evResp
	}
	runCoordinates := job.RunCoordinates{JobID: jobID, RunID: runID}
	runStatus, err := jm.jobRunner.BuildRunStatus(runCoordinates, currentJob)
	if err != nil {
		evResp.Err = fmt.Errorf("could not rebuild the status of the job: %v", err)
		return &evResp
	}
	jobStatus.RunStatus = *runStatus
	evResp.Status = &jobStatus
	evResp.Err = nil
	return &evResp
}
