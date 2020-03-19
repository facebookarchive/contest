// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

// TargetRoutingEvents gather all event names which track the flow of targets
// between TestSteps
var TargetRoutingEvents = []event.Name{
	target.EventTargetIn,
	target.EventTargetErr,
	target.EventTargetOut,
	target.EventTargetInErr,
}

// buildTargetStatuses builds a list of TargetStepStatus, which represent the status of Targets within a TestStep
func (jr *JobRunner) buildTargetStatuses(coordinates job.TestStepCoordinates) ([]job.TargetStatus, error) {

	// Fetch all the events associated to Targets routing
	routingEvents, err := jr.testEvManager.Fetch(
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

		// Update the TargetStatus object associated to the Target. If there is no TargetStatus associated yet, append it
		var targetStatus *job.TargetStatus
		for index, candidateStatus := range targetStatuses {
			if *candidateStatus.Target == *testEvent.Data.Target {
				targetStatus = &targetStatuses[index]
				break
			}
		}

		if targetStatus == nil {
			// There is no TargetStatus associated with this Target, create one
			targetStatuses = append(targetStatuses, job.TargetStatus{TestStepCoordinates: coordinates, Target: testEvent.Data.Target})
			targetStatus = &targetStatuses[len(targetStatuses)-1]
		}

		evName := testEvent.Data.EventName
		if evName == target.EventTargetIn {
			targetStatus.InTime = testEvent.EmitTime
		} else if evName == target.EventTargetOut {
			targetStatus.OutTime = testEvent.EmitTime
		} else if evName == target.EventTargetErr {
			targetStatus.OutTime = testEvent.EmitTime
			errorPayload := target.ErrPayload{}
			jsonPayload, err := testEvent.Data.Payload.MarshalJSON()
			if err != nil {
				targetStatus.Error = fmt.Sprintf("could not marshal payload error: %v", err)
			} else {
				if err := json.Unmarshal(jsonPayload, &errorPayload); err != nil {
					targetStatus.Error = fmt.Sprintf("could not unmarshal payload error: %v", err)
				} else {
					targetStatus.Error = errorPayload.Error
				}
			}
		}
	}

	return targetStatuses, nil
}

// buildTestStepStatus builds the status object of a test step belonging to a test
func (jr *JobRunner) buildTestStepStatus(coordinates job.TestStepCoordinates) (*job.TestStepStatus, error) {

	testStepStatus := job.TestStepStatus{TestStepCoordinates: coordinates}

	// Fetch all Events associated to this TestStep
	testEvents, err := jr.testEvManager.Fetch(
		testevent.QueryJobID(coordinates.JobID),
		testevent.QueryRunID(coordinates.RunID),
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
	targetStatuses, err := jr.buildTargetStatuses(coordinates)
	if err != nil {
		return nil, fmt.Errorf("could not build target status for test step %s: %v", coordinates.TestStepLabel, err)
	}
	testStepStatus.TargetStatuses = targetStatuses
	return &testStepStatus, nil
}

// buildTestStatus builds the status of a test belonging to a specific to a test
func (jr *JobRunner) buildTestStatus(coordinates job.TestCoordinates, currentJob *job.Job) (*job.TestStatus, error) {

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
		TestCoordinates:  coordinates,
		TestStepStatuses: make([]job.TestStepStatus, len(currentTest.TestStepsBundles)),
	}

	// Build a TestStepStatus object for each TestStep
	for index, bundle := range currentTest.TestStepsBundles {
		testStepCoordinates := job.TestStepCoordinates{
			TestCoordinates: coordinates,
			TestStepName:    bundle.TestStep.Name(),
			TestStepLabel:   bundle.TestStepLabel,
		}
		testStepStatus, err := jr.buildTestStepStatus(testStepCoordinates)
		if err != nil {
			return nil, fmt.Errorf("could not build TestStatus for test %s: %v", bundle.TestStep.Name(), err)
		}
		testStatus.TestStepStatuses[index] = *testStepStatus
	}

	// Calculate the overall status of the Targets which corresponds to the last TargetStatus
	// object recorded for each Target.

	// Fetch all events signaling that a Target has been acquired. This is the source of truth
	// indicating which Targets belong to a Test.
	targetAcquiredEvents, err := jr.testEvManager.Fetch(
		testevent.QueryJobID(coordinates.JobID),
		testevent.QueryRunID(coordinates.RunID),
		testevent.QueryTestName(coordinates.TestName),
		testevent.QueryEventName(target.EventTargetAcquired),
	)

	if err != nil {
		return nil, fmt.Errorf("could not fetch events associated to target acquisition")
	}

	var targetStatuses []job.TargetStatus

	// Keep track of the last TargetStatus seen for each Target
	targetMap := make(map[target.Target]job.TargetStatus)
	for _, testStepStatus := range testStatus.TestStepStatuses {
		for _, targetStatus := range testStepStatus.TargetStatuses {
			targetMap[*targetStatus.Target] = targetStatus
		}
	}

	for _, targetEvent := range targetAcquiredEvents {
		t := *targetEvent.Data.Target
		if _, ok := targetMap[t]; !ok {
			// This Target is not associated to any TargetStatus, we assume it has not
			// started the test
			targetMap[t] = job.TargetStatus{}
		}
		targetStatuses = append(targetStatuses, targetMap[t])
	}

	testStatus.TargetStatuses = targetStatuses
	return &testStatus, nil
}

// BuildRunStatus builds the status of a run with a job
func (jr *JobRunner) BuildRunStatus(coordinates job.RunCoordinates, currentJob *job.Job) (*job.RunStatus, error) {

	runStatus := job.RunStatus{RunCoordinates: coordinates, TestStatuses: make([]job.TestStatus, len(currentJob.Tests))}

	for index, currentTest := range currentJob.Tests {
		testCoordinates := job.TestCoordinates{RunCoordinates: coordinates, TestName: currentTest.Name}
		testStatus, err := jr.buildTestStatus(testCoordinates, currentJob)
		if err != nil {
			return nil, fmt.Errorf("could not rebuild status for test %s: %v", currentTest.Name, err)
		}
		runStatus.TestStatuses[index] = *testStatus
	}
	return &runStatus, nil
}

// BuildRunStatuses builds the status of all runs belonging to the job
func (jr *JobRunner) BuildRunStatuses(currentJob *job.Job) ([]job.RunStatus, error) {

	// Calculate the status only for the runs which effectively were executed
	runStartEvents, err := jr.frameworkEventManager.Fetch(frameworkevent.QueryEventName(EventRunStarted))
	if err != nil {
		return nil, fmt.Errorf("could not determine how many runs were executed: %v", err)
	}
	numRuns := uint(0)
	if len(runStartEvents) == 0 {
		return make([]job.RunStatus, 0, currentJob.Runs), nil
	}

	payload, err := runStartEvents[len(runStartEvents)-1].Payload.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("could not extract JSON payload from RunStart event: %v", err)
	}

	payloadUnmarshaled := RunStartedPayload{}

	if err := json.Unmarshal(payload, &payloadUnmarshaled); err != nil {
		return nil, fmt.Errorf("could not unmarshal RunStarted event payload")
	}
	numRuns = uint(payloadUnmarshaled.RunID)

	runStatuses := make([]job.RunStatus, 0, numRuns)

	for runID := uint(1); runID <= numRuns; runID++ {
		runCoordinates := job.RunCoordinates{JobID: currentJob.ID, RunID: types.RunID(runID)}
		runStatus, err := jr.BuildRunStatus(runCoordinates, currentJob)
		if err != nil {
			return nil, fmt.Errorf("could not rebuild run status for run %d: %v", runID, err)
		}
		runStatuses = append(runStatuses, *runStatus)
	}
	return runStatuses, nil
}
