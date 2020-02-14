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

// JobCompletionEvents gather all event that mark the end of a job
var JobCompletionEvents = []event.Name{
	EventJobCompleted,
	EventJobFailed,
	EventJobCancelled,
	EventJobCancellationFailed,
}

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

// RunCoordinates collects information to identify the run for which we want to rebuild the status
type RunCoordinates struct {
	JobID types.JobID
	RunID types.RunID
}

// TestCoordinates collects information to identify the test for which we want to rebuild the status
type TestCoordinates struct {
	RunCoordinates
	TestName string
}

// TestStepCoordinates collects information to identify the test step for which we want to rebuild the status
type TestStepCoordinates struct {
	TestCoordinates
	TestStepName  string
	TestStepLabel string
}

// buildTargetStatus populates a TestStepStatus object with TestStepStatus information
func (jm *JobManager) buildTargetStatuses(coordinates TestStepCoordinates) ([]job.TargetStatus, error) {

	// Fetch all the events associated to Targets routing
	routingEvents, err := jm.testEvManager.Fetch(
		testevent.QueryJobID(coordinates.JobID),
		testevent.QueryTestName(coordinates.TestName),
		testevent.QueryTestStepLabel(coordinates.TestStepLabel),
		testevent.QueryEventNames(TargetRoutingEvents),
	)
	if err != nil {
		return nil, fmt.Errorf("could not fetch events associated to target routing: %v", err)
	}

	var targetStatuses []job.TargetStatus
	for _, testEvent := range routingEvents {
		// Update the TargetStatus object associated to the Target.
		// If there is no TargetStatus associated yet, append it
		var targetStatus *job.TargetStatus
		for index, candidateStatus := range targetStatuses {
			if *candidateStatus.Target == *testEvent.Data.Target {
				targetStatus = &targetStatuses[index]
				break
			}
		}

		if targetStatus == nil {
			// There is no TargetStatus associated with this Target, create one
			targetStatuses = append(targetStatuses, job.TargetStatus{Target: testEvent.Data.Target})
			targetStatus = &targetStatuses[len(targetStatuses)-1]
		}

		if testEvent.Data.EventName == target.EventTargetIn {
			targetStatus.InTime = testEvent.EmitTime
		} else if testEvent.Data.EventName == target.EventTargetOut {
			targetStatus.OutTime = testEvent.EmitTime
		} else if testEvent.Data.EventName == target.EventTargetErr {
			targetStatus.OutTime = testEvent.EmitTime
			targetStatus.Error = testEvent.Data.Payload
		}
	}

	return targetStatuses, nil
}

// buildTestStepStatus builds the status object of a test step belonging to a test
func (jm *JobManager) buildTestStepStatus(coordinates TestStepCoordinates) (*job.TestStepStatus, error) {

	testStepStatus := job.TestStepStatus{TestStepName: coordinates.TestStepName, TestStepLabel: coordinates.TestStepLabel}

	// Fetch all Events associated to this TestStep
	// TODO: This query will have to include the runID, which can be found in the coordinates
	testEvents, err := jm.testEvManager.Fetch(
		testevent.QueryJobID(coordinates.JobID),
		testevent.QueryTestName(coordinates.TestName),
		testevent.QueryTestStepLabel(coordinates.TestStepLabel),
	)
	if err != nil {
		return nil, fmt.Errorf("could not fetch events associated to test step %s: %v", coordinates.TestStepLabel, err)
	}

	// Build a map of events that should not be rendered as part of the status (e.g. routing events)
	skipEvents := make(map[event.Name]struct{})
	for _, eventName := range TargetRoutingEvents {
		skipEvents[eventName] = struct{}{}
	}
	filteredTestEvents := []testevent.Event{}
	for _, event := range testEvents {
		if _, skip := skipEvents[event.Data.EventName]; !skip {
			filteredTestEvents = append(filteredTestEvents, event)
		}
	}

	testStepStatus.Events = filteredTestEvents
	targetStatuses, err := jm.buildTargetStatuses(coordinates)
	if err != nil {
		return nil, fmt.Errorf("could not build target status for test step %s: %v", coordinates.TestStepLabel, err)
	}
	testStepStatus.TargetStatuses = targetStatuses
	return &testStepStatus, nil
}

// buildTestStatus builds the status of a test belonging to a specific to a test
func (jm *JobManager) buildTestStatus(coordinates TestCoordinates, currentJob *job.Job) (*job.TestStatus, error) {

	var currentTest *test.Test
	// Identify the test within the Job for which we are asking to calculate the status
	for _, candidateTest := range currentJob.Tests {
		if candidateTest.Name == coordinates.TestName {
			currentTest = candidateTest
			break
		}
	}

	if currentTest == nil {
		return nil, fmt.Errorf("job with id %d does not include any test named %s", coordinates.JobID, coordinates.TestName)
	}
	testStatus := job.TestStatus{
		TestName:         coordinates.TestName,
		TestStepStatuses: make([]job.TestStepStatus, len(currentTest.TestStepsBundles)),
	}

	// Build a TestStepStatus object for each TestStep
	for index, bundle := range currentTest.TestStepsBundles {
		testStepCoordinates := TestStepCoordinates{
			TestCoordinates: coordinates,
			TestStepName:    bundle.TestStep.Name(),
			TestStepLabel:   bundle.TestStepLabel,
		}
		status, err := jm.buildTestStepStatus(testStepCoordinates)
		if err != nil {
			return nil, fmt.Errorf("could not build TestStatus for test %s: %v", bundle.TestStep.Name(), err)
		}
		testStatus.TestStepStatuses[index] = *status
	}
	return &testStatus, nil
}

// BuildRunStatus builds the status of a run with a job
func (jm *JobManager) BuildRunStatus(coordinates RunCoordinates, currentJob *job.Job) (*job.RunStatus, error) {

	runStatus := job.RunStatus{RunID: coordinates.RunID, TestStatuses: make([]job.TestStatus, len(currentJob.Tests))}

	for index, currentTest := range currentJob.Tests {
		testCoordinates := TestCoordinates{RunCoordinates: coordinates, TestName: currentTest.Name}
		testStatus, err := jm.buildTestStatus(testCoordinates, currentJob)
		if err != nil {
			return nil, fmt.Errorf("could not rebuild status for test %s: %v", currentTest.Name, err)
		}
		runStatus.TestStatuses[index] = *testStatus
	}
	return &runStatus, nil
}

// BuildStatus builds the status of all runs belonging to the job
func (jm *JobManager) BuildStatus(currentJob *job.Job) ([]job.RunStatus, error) {

	runStatuses := make([]job.RunStatus, 0, currentJob.Runs)
	for runID := uint(0); runID < currentJob.Runs; runID++ {
		runCoordinates := RunCoordinates{JobID: currentJob.ID, RunID: types.RunID(runID)}
		runStatus, err := jm.BuildRunStatus(runCoordinates, currentJob)
		if err != nil {
			return nil, fmt.Errorf("could not rebuild run status for run %d: %v", runID, err)
		}
		runStatuses[runID] = *runStatus
	}
	return runStatuses, nil
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
	runCoordinates := RunCoordinates{JobID: jobID, RunID: runID}
	runStatus, err := jm.BuildRunStatus(runCoordinates, currentJob)
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
