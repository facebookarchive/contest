// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package memory

import (
	"fmt"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
)

// Memory implements a storage engine which stores everything in memory. This
// storage engine is very inefficient and should be used only for testing
// purposes.
type Memory struct {
	lock            *sync.Mutex
	testEvents      []testevent.Event
	frameworkEvents []frameworkevent.Event
	jobIDCounter    types.JobID
	jobRequests     map[types.JobID]*job.Request
	jobReports      map[types.JobID]*job.JobReport
}

func emptyEventQuery(eventQuery *event.Query) bool {
	return eventQuery.JobID == 0 && len(eventQuery.EventNames) == 0 && eventQuery.EmittedStartTime.IsZero() && eventQuery.EmittedEndTime.IsZero()

}

// emptyFrameworkEventQuery returns whether the Query contains only default values
// If so, the Query is considered "empty" and doesn't result in any lookup in the
// database
func emptyFrameworkEventQuery(eventQuery *frameworkevent.Query) bool {
	return emptyEventQuery(&eventQuery.Query)
}

// emptyTestEventQuery returns whether the Query contains only default
// values. If so, the Query is considered "empty" and doesn't result in
// any lookup in the database
func emptyTestEventQuery(eventQuery *testevent.Query) bool {
	return emptyEventQuery(&eventQuery.Query) && eventQuery.TestName == "" && eventQuery.TestStepLabel == ""
}

// StoreTestEvent stores a test event into the database
func (m *Memory) StoreTestEvent(event testevent.Event) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.testEvents = append(m.testEvents, event)
	return nil
}

func eventJobMatch(queryJobID types.JobID, jobID types.JobID) bool {
	if queryJobID != 0 && jobID != queryJobID {
		return false
	}
	return true
}

func eventNameMatch(queryEventNames []event.Name, eventName event.Name) bool {
	if len(queryEventNames) == 0 {
		// If no criteria was specified for matching the name of the event,
		// do not filter it out
		return true
	}
	for _, candidateEventName := range queryEventNames {
		if eventName == candidateEventName {
			return true
		}
	}
	return false
}

func eventTimeMatch(queryStartTime, queryEndTime time.Time, emittedTime time.Time) bool {
	if !queryStartTime.IsZero() && queryStartTime.Sub(emittedTime) > 0 {
		return false
	}
	if !queryEndTime.IsZero() && emittedTime.Sub(queryEndTime) > 0 {
		return false
	}
	return true
}

func eventTestMatch(queryTestName, testName string) bool {
	if queryTestName != "" && testName != queryTestName {
		return false
	}
	return true
}

func eventTestStepMatch(queryTestStepLabel, testStepLabel string) bool {
	if queryTestStepLabel != "" && queryTestStepLabel != testStepLabel {
		return false
	}
	return true
}

// GetTestEvents returns all test events that match the given query.
func (m *Memory) GetTestEvents(eventQuery *testevent.Query) ([]testevent.Event, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var matchingTestEvents []testevent.Event

	if emptyTestEventQuery(eventQuery) {
		return matchingTestEvents, nil
	}

	for _, event := range m.testEvents {
		if eventJobMatch(eventQuery.JobID, event.Header.JobID) &&
			eventNameMatch(eventQuery.EventNames, event.Data.EventName) &&
			eventTimeMatch(eventQuery.EmittedStartTime, eventQuery.EmittedEndTime, event.EmitTime) &&
			eventTestMatch(eventQuery.TestName, event.Header.TestName) &&
			eventTestStepMatch(eventQuery.TestStepLabel, event.Header.TestStepLabel) {
			matchingTestEvents = append(matchingTestEvents, event)
		}
	}
	return matchingTestEvents, nil
}

// Reset restores the original state of the memory storage layer
func (m *Memory) Reset() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.testEvents = []testevent.Event{}
	m.frameworkEvents = []frameworkevent.Event{}
	m.jobRequests = make(map[types.JobID]*job.Request)
	m.jobReports = make(map[types.JobID]*job.JobReport)
	m.jobIDCounter = 1
	return nil
}

// StoreJobRequest stores a new job request
func (m *Memory) StoreJobRequest(request *job.Request) (types.JobID, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	request.JobID = m.jobIDCounter
	m.jobIDCounter++
	m.jobRequests[request.JobID] = request
	return request.JobID, nil
}

// GetJobRequest retrieves a job request from the in memory list
func (m *Memory) GetJobRequest(jobID types.JobID) (*job.Request, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	v, ok := m.jobRequests[jobID]
	if !ok {
		return nil, fmt.Errorf("could not find job request with id %v", jobID)
	}
	return v, nil
}

// StoreJobReport stores a report associated to a job. Returns an error if there is
// already a report associated to the job
func (m *Memory) StoreJobReport(report *job.JobReport) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	_, ok := m.jobReports[report.JobID]
	if ok {
		return fmt.Errorf("job report already present for job id %v", report.JobID)
	}
	m.jobReports[report.JobID] = report
	return nil
}

// GetJobReport returns the report associated to a given job
func (m *Memory) GetJobReport(jobID types.JobID) (*job.JobReport, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if _, ok := m.jobReports[jobID]; !ok {
		// return a job report with no results
		return &job.JobReport{JobID: jobID}, nil
	}
	return m.jobReports[jobID], nil
}

// StoreFrameworkEvent stores a framework event into the database
func (m *Memory) StoreFrameworkEvent(event frameworkevent.Event) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.frameworkEvents = append(m.frameworkEvents, event)
	return nil
}

// GetFrameworkEvent retrieves a framework event from storage
func (m *Memory) GetFrameworkEvent(eventQuery *frameworkevent.Query) ([]frameworkevent.Event, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var matchingFrameworkEvents []frameworkevent.Event

	if emptyFrameworkEventQuery(eventQuery) {
		return matchingFrameworkEvents, nil
	}
	for _, event := range m.frameworkEvents {
		if eventJobMatch(eventQuery.JobID, event.JobID) &&
			eventNameMatch(eventQuery.EventNames, event.EventName) &&
			eventTimeMatch(eventQuery.EmittedStartTime, eventQuery.EmittedEndTime, event.EmitTime) {
			matchingFrameworkEvents = append(matchingFrameworkEvents, event)
		}
	}
	return matchingFrameworkEvents, nil
}

// New create a new Memory events storage backend
func New() (storage.Storage, error) {
	m := Memory{lock: &sync.Mutex{}}
	m.jobRequests = make(map[types.JobID]*job.Request)
	m.jobReports = make(map[types.JobID]*job.JobReport)
	m.jobIDCounter = 1
	return &m, nil
}
