// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package memory

import (
	"encoding/json"
	"fmt"
	"sort"
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
	jobInfo         map[types.JobID]*jobInfo
}

type jobInfo struct {
	request *job.Request
	desc    *job.Descriptor
	report  *job.JobReport
	state   job.State
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

func eventRunMatch(queryRunID, runID types.RunID) bool {
	if queryRunID != 0 && runID != queryRunID {
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
			eventRunMatch(eventQuery.RunID, event.Header.RunID) &&
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
	m.jobInfo = make(map[types.JobID]*jobInfo)
	m.jobIDCounter = 1
	return nil
}

// StoreJobRequest stores a new job request
func (m *Memory) StoreJobRequest(request *job.Request) (types.JobID, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	jobID := m.jobIDCounter
	m.jobIDCounter++
	request.JobID = jobID
	info := &jobInfo{
		request: request,
		desc:    &job.Descriptor{},
		state:   job.JobStateUnknown,
	}
	if err := json.Unmarshal([]byte(request.JobDescriptor), info.desc); err != nil {
		return 0, fmt.Errorf("invalid job descriptor: %w", err)
	}
	m.jobInfo[jobID] = info
	return jobID, nil
}

// GetJobRequest retrieves a job request from the in memory list
func (m *Memory) GetJobRequest(jobID types.JobID) (*job.Request, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	v := m.jobInfo[jobID]
	if v == nil || v.request == nil {
		return nil, fmt.Errorf("could not find job request with id %v", jobID)
	}
	return v.request, nil
}

// StoreJobReport stores a report associated to a job. Returns an error if there is
// already a report associated to the job
func (m *Memory) StoreJobReport(report *job.JobReport) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	v := m.jobInfo[report.JobID]
	if v == nil {
		return fmt.Errorf("could not find job with id %v", report.JobID)
	}
	if v.report != nil {
		return fmt.Errorf("job report already present for job id %v", report.JobID)
	}
	v.report = report
	return nil
}

// GetJobReport returns the report associated to a given job
func (m *Memory) GetJobReport(jobID types.JobID) (*job.JobReport, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	v := m.jobInfo[jobID]
	if v == nil || v.report == nil {
		// return a job report with no results
		return &job.JobReport{JobID: jobID}, nil
	}
	return v.report, nil
}

func (m *Memory) ListJobs(query *storage.JobQuery) ([]types.JobID, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	res := []types.JobID{}
	if err := job.CheckTags(query.Tags, true /* allowInternal */); err != nil {
		return nil, err
	}
jobLoop:
	for jobId, jobInfo := range m.jobInfo {
		if len(query.Tags) > 0 {
			for _, qTag := range query.Tags {
				found := false
				for _, jTag := range jobInfo.desc.Tags {
					if jTag == qTag {
						found = true
					}
				}
				if !found {
					continue jobLoop
				}
			}
		}
		if len(query.States) > 0 {
			var lastEventTime time.Time
			jobState := job.JobStateUnknown
			for _, event := range m.frameworkEvents {
				if eventJobMatch(jobId, event.JobID) &&
					eventNameMatch(job.JobStateEvents, event.EventName) &&
					event.EmitTime.After(lastEventTime) {
					jobState, _ = job.EventNameToJobState(event.EventName)
					lastEventTime = event.EmitTime
				}
			}
			found := false
			for _, queryState := range query.States {
				if jobState == queryState {
					found = true
					break
				}
			}
			if !found {
				continue jobLoop
			}
		}
		res = append(res, jobId)
	}
	sort.Slice(res, func(i, j int) bool { return res[i] < res[j] })
	return res, nil
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

// Version returns the version of the memory storage layer.
func (m *Memory) Version() (uint64, error) {
	return 0, nil
}

// New create a new Memory events storage backend
func New() (storage.Storage, error) {
	m := Memory{lock: &sync.Mutex{}}
	m.jobInfo = make(map[types.JobID]*jobInfo)
	m.jobIDCounter = 1
	return &m, nil
}
