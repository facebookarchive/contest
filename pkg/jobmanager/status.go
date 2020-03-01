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
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
)

func (jm *JobManager) status(ev *api.Event) *api.EventResponse {
	msg := ev.Msg.(api.EventStatusMsg)
	jobID := msg.JobID

	report, err := jm.jobReportManager.Fetch(jobID)
	if err != nil {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not fetch job report: %v", err),
		}
	}

	// Fetch all the events associated to changes of state of the Job
	jobEvents, err := jm.frameworkEvManager.Fetch(
		frameworkevent.QueryJobID(jobID),
		frameworkevent.QueryEventNames(JobStateEvents),
	)
	if err != nil {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not fetch events associated to job state: %v", err),
		}
	}
	currentJob, ok := jm.jobs[jobID]
	if !ok {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("job manager is not owner of job id: %v", jobID),
		}
	}

	// Lookup job starting time and job termination time based on the events emitted
	var (
		startTime time.Time
		endTime   time.Time
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
			if !endTime.IsZero() {
				log.Warningf("Job %d is associated to multiple completion events", jobID)
			}
			endTime = ev.EmitTime
		}
	}

	state := "Unknown"
	if len(jobEvents) > 0 {
		eventName := jobEvents[len(jobEvents)-1].EventName
		state = string(eventName)
	}

	jobStatus := job.Status{Name: currentJob.Name, StartTime: startTime, EndTime: endTime, State: state, JobReport: report}

	// Fetch the ID of the last run that was started
	runID, err := jm.jobRunner.GetCurrentRun(jobID)
	if err != nil {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not determine the current run id being executed: %v", err),
		}

	}
	runCoordinates := job.RunCoordinates{JobID: jobID, RunID: runID}
	runStatus, err := jm.jobRunner.BuildRunStatus(runCoordinates, currentJob)
	if err != nil {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not rebuild the status of the job: %v", err),
		}
	}
	jobStatus.RunStatus = *runStatus
	return &api.EventResponse{
		JobID:     jobID,
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
		Status:    &jobStatus,
	}
}
