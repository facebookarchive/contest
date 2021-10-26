// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
)

func (jm *JobManager) status(ev *api.Event) *api.EventResponse {
	ctx := storage.WithConsistencyModel(ev.Context, storage.ConsistentEventually)

	msg := ev.Msg.(api.EventStatusMsg)
	jobID := msg.JobID
	evResp := api.EventResponse{
		JobID:     jobID,
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
	}

	// Look up job request.
	req, err := jm.jsm.GetJobRequest(ctx, jobID)
	if err != nil {
		evResp.Err = fmt.Errorf("failed to fetch request for job ID %d: %w", jobID, err)
		return &evResp
	}

	currentJob, err := NewJobFromExtendedDescriptor(ctx, jm.pluginRegistry, req.ExtendedDescriptor)
	if err != nil {
		evResp.Err = fmt.Errorf("failed to build job object from job request: %w", err)
		return &evResp
	}

	// currentJob temporary object is just used as an interface to the job extended descriptor
	// so populate it with the other necessary fields such as id (currently 0)
	currentJob.ID = jobID

	// Is it for our instance?
	if jm.config.instanceTag != "" {
		found := false
		for _, tag := range currentJob.Tags {
			if tag == jm.config.instanceTag {
				found = true
				break
			}
		}
		if !found {
			evResp.Err = fmt.Errorf("job %d belongs to a different instance, this is %q",
				jobID, jm.config.instanceTag)
			return &evResp
		}
	}

	// Fetch all the events associated to changes of state of the Job
	jobEvents, err := jm.frameworkEvManager.Fetch(ctx,
		frameworkevent.QueryJobID(jobID),
		frameworkevent.QueryEventNames(job.JobStateEvents),
	)
	if err != nil {
		evResp.Err = fmt.Errorf("could not fetch events associated to job state: %v", err)
		return &evResp
	}

	// Lookup job starting time and job termination time based on the events emitted
	var (
		startTime time.Time
		endTime   *time.Time
	)

	completionEvents := make(map[event.Name]bool)
	for _, eventName := range job.JobCompletionEvents {
		completionEvents[eventName] = true
	}

	for _, ev := range jobEvents {
		if ev.EventName == job.EventJobStarted {
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
		if je.EventName == job.EventJobFailed {
			// if there was a framework failure, retrieve the failure event and
			// the associated error message, so it can be exposed in the status.
			if je.Payload == nil {
				stateErrMsg = "internal error: EventJobFailed's payload is nil"
			} else {
				var ep ErrorEventPayload
				if err := json.Unmarshal(*je.Payload, &ep); err != nil {
					stateErrMsg = fmt.Sprintf("internal error: EventJobFailed's payload cannot be unmarshalled. Raw payload: %s, Error: %v", *je.Payload, err)
				} else {
					stateErrMsg = ep.Err.Error()
				}
			}
		}
	}

	report, err := jm.jsm.GetJobReport(ctx, jobID)
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

	jobStatus.RunStatuses, err = jm.jobRunner.BuildRunStatuses(ctx, currentJob)
	if err != nil {
		evResp.Err = fmt.Errorf("could not rebuild the statuses of the job: %v", err)
		return &evResp
	}

	if len(jobStatus.RunStatuses) > 0 {
		// NOTE: deprecated, keeping for backwards compat
		jobStatus.RunStatus = &jobStatus.RunStatuses[len(jobStatus.RunStatuses)-1]
	}

	evResp.Status = &jobStatus
	evResp.Err = nil
	return &evResp
}
