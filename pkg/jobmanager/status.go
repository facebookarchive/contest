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
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

// JobStateEvents gather all event names which track the state of a job
var JobStateEvents = []event.Name{
	EventJobStarted,
	EventJobCompleted,
	EventJobFailed,
	EventJobCancelling,
	EventJobCancelled,
	EventJobCancellationFailed,
}

// TargetRoutingEvents gather all event names which track the flow of targets
// between TestSteps
var TargetRoutingEvents = []event.Name{
	target.EventTargetIn,
	target.EventTargetErr,
	target.EventTargetOut,
	target.EventTargetInErr,
}

// buildTargetStatus populates a TestStepStatus object with TestStepStatus information
func (jm *JobManager) buildTargetStatus(jobID types.JobID, currentTestStepStatus *job.TestStepStatus) error {

	// Fetch all the events associated to Targets routing
	routingEvents, err := jm.testEvManager.Fetch(
		[]testevent.QueryField{
			testevent.QueryJobID(jobID),
			testevent.QueryEventNames(TargetRoutingEvents),
		},
	)
	if err != nil {
		return fmt.Errorf("could not fetch events associated to target routing: %v", err)
	}

	// Filter only the events emitted by this TestStep, group events by Target
	for _, testEvent := range routingEvents {
		if testEvent.Header.TestStepLabel != currentTestStepStatus.TestStepLabel {
			continue
		}
		// Update the TargetStatus object associated to the Target. If there is no
		// TargetStatus associated yet, append it
		var currentTargetStatus *job.TargetStatus
		for index, candidateTargetStatus := range currentTestStepStatus.TargetStatus {
			if *candidateTargetStatus.Target == *testEvent.Data.Target {
				currentTargetStatus = &currentTestStepStatus.TargetStatus[index]
				break
			}
		}
		if currentTargetStatus == nil {
			// There is no TargetStatus associated with this Target, create one
			currentTestStepStatus.TargetStatus = append(
				currentTestStepStatus.TargetStatus,
				job.TargetStatus{Target: testEvent.Data.Target},
			)
			lastTargetStatus := len(currentTestStepStatus.TargetStatus) - 1
			currentTargetStatus = &currentTestStepStatus.TargetStatus[lastTargetStatus]
		}

		switch eventName := testEvent.Data.EventName; eventName {
		case target.EventTargetIn:
			currentTargetStatus.InTime = testEvent.EmitTime
		case target.EventTargetOut:
			currentTargetStatus.OutTime = testEvent.EmitTime
		case target.EventTargetErr:
			currentTargetStatus.OutTime = testEvent.EmitTime
			currentTargetStatus.Error = testEvent.Data.Payload
		}
	}

	return nil
}

// buildTestStepStatus populates a TestStatus object with TestStep status information
func (jm *JobManager) buildTestStepStatus(jobID types.JobID, currentTest *test.Test, currentTestStatus *job.TestStatus) error {

	// Build a map of events that should not be rendered as part of the status
	skipEvents := make(map[event.Name]bool)
	for _, eventName := range TargetRoutingEvents {
		skipEvents[eventName] = true
	}

	// Go through every bundle describing a TestStep and rebuild the status of that TestStep
	for _, testStepBundle := range currentTest.TestStepsBundles {
		// Fetch all Events associated to this TestStep and render them as part of the
		testEvents, err := jm.testEvManager.Fetch(
			[]testevent.QueryField{
				testevent.QueryJobID(jobID),
				testevent.QueryTestStepLabel(testStepBundle.TestStepLabel),
			},
		)
		if err != nil {
			return fmt.Errorf("could not fetch events associated to TestStep: %v", err)
		}

		// Filter out events associated to the TestStep that were emitted by the framework
		// (e.g. routing events)
		filteredTestEvent := []testevent.Event{}
		for _, event := range testEvents {
			if _, skip := skipEvents[event.Data.EventName]; !skip {
				filteredTestEvent = append(filteredTestEvent, event)
			}
		}
		currentTestStatus.TestStepStatus = append(
			currentTestStatus.TestStepStatus,
			job.TestStepStatus{
				TestStepName:  testStepBundle.TestStep.Name(),
				TestStepLabel: testStepBundle.TestStepLabel,
				Events:        filteredTestEvent,
				TargetStatus:  make([]job.TargetStatus, 0),
			},
		)
		last := len(currentTestStatus.TestStepStatus) - 1
		currenTestStepStatus := &currentTestStatus.TestStepStatus[last]

		if err := jm.buildTargetStatus(jobID, currenTestStepStatus); err != nil {
			return err
		}
	}
	return nil
}

// buildTestStatus populates a JobStatus object with TestStatus information
func (jm *JobManager) buildTestStatus(jobID types.JobID, tests []*test.Test, jobStatus *job.Status) error {
	for _, test := range tests {
		jobStatus.TestStatus = append(
			jobStatus.TestStatus,
			job.TestStatus{
				TestName:       test.Name,
				TestStepStatus: make([]job.TestStepStatus, 0, len(test.TestStepsBundles)),
			},
		)
		currentTestStatus := &jobStatus.TestStatus[len(jobStatus.TestStatus)-1]

		if err := jm.buildTestStepStatus(jobID, test, currentTestStatus); err != nil {
			return err
		}
	}
	return nil
}

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

	// TODO: JobManager relies only on events to rebuild the status of the job.
	// It should fetch a complete list of targets, and render the progress based
	// on events instead.

	// Fetch all the events associated to changes of state of the Job
	jobEvents, err := jm.frameworkEvManager.Fetch(
		[]frameworkevent.QueryField{
			frameworkevent.QueryJobID(jobID),
			frameworkevent.QueryEventNames(JobStateEvents),
		},
	)
	if err != nil {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not fetch events associated to job state: %v", err),
		}
	}
	currentJob, ok := jm.jobs[jobID]
	// BUG If we return an error via the API, the HTTP listener simply returns a nil response?
	if !ok {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("job manager is not owned of job id: %v", jobID),
		}
	}

	// Lookup job starting time and job termination time (i.e. when the job report was built)
	var (
		startTime  time.Time
		reportTime time.Time
	)
	if report != nil {
		reportTime = report.ReportTime
	}
	for _, ev := range jobEvents {
		if ev.EventName == EventJobStarted {
			startTime = ev.EmitTime
		}
	}
	jobStatus := job.Status{
		Name:       currentJob.Name,
		StartTime:  startTime,
		EndTime:    reportTime,
		Report:     report,
		TestStatus: make([]job.TestStatus, 0, len(currentJob.Tests)),
	}

	if err := jm.buildTestStatus(jobID, currentJob.Tests, &jobStatus); err != nil {
		return &api.EventResponse{
			JobID:     jobID,
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("could not rebuild the status of the job: %v", err),
		}
	}

	return &api.EventResponse{
		JobID:     jobID,
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
		Status:    &jobStatus,
	}
}
