// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package testevent

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/internal/querytools"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// Header models the header of a test event, which consists in metadatat hat defines the
// emitter of the events. The Header is under ConTest control and cannot be manipulated
// by the TestStep
type Header struct {
	JobID         types.JobID
	RunID         types.RunID
	TestName      string
	TestStepLabel string
}

// Data models the data of a test event. It is populated by the TestStep
type Data struct {
	EventName event.Name
	Target    *target.Target
	Payload   *json.RawMessage
}

// Event models an event object that can be emitted by a TestStep
type Event struct {
	EmitTime time.Time
	Header   *Header
	Data     *Data
}

// New creates a new Event with zero value header and data
func New(header *Header, data *Data) Event {
	return Event{Header: header, Data: data}
}

// Query wraps information that are used to build queries for
// test events, on top of the common EventQuery fields
type Query struct {
	event.Query
	RunID         types.RunID
	TestName      string
	TestStepLabel string
}

// QueryField defines a function type used to set a field's value on Query objects
type QueryField interface {
	queryFieldPointer(query *Query) interface{}
}

// QueryFields is a set of field values for a Query object
type QueryFields []QueryField

// BuildQuery compiles a Query from scratch using values of queryFields.
// It does basically just creates an empty query an applies queryFields to it.
func (queryFields QueryFields) BuildQuery() (*Query, error) {
	query := &Query{}
	for idx, queryField := range queryFields {
		if err := querytools.ApplyQueryField(queryField.queryFieldPointer(query), queryField); err != nil {
			return nil, fmt.Errorf("unable to apply field %d:%T(%v): %w", idx, queryField, queryField, err)
		}
	}
	return query, nil
}

// BuildQuery compiles a Query from scratch using values of queryFields.
// It does basically just creates an empty query an applies queryFields to it.
func BuildQuery(queryFields ...QueryField) (*Query, error) {
	return QueryFields(queryFields).BuildQuery()
}

type queryFieldJobID types.JobID
type queryFieldEventNames []event.Name
type queryFieldEmittedStartTime time.Time
type queryFieldEmittedEndTime time.Time
type queryFieldTestName string
type queryFieldTestStepLabel string

// QueryJobID sets the JobID field of the Query object
func QueryJobID(jobID types.JobID) QueryField                            { return queryFieldJobID(jobID) }
func (value queryFieldJobID) queryFieldPointer(query *Query) interface{} { return &query.JobID }

// QueryEventNames the EventNames field of the Query object
func QueryEventNames(eventNames []event.Name) QueryField { return queryFieldEventNames(eventNames) }
func (value queryFieldEventNames) queryFieldPointer(query *Query) interface{} {
	return &query.EventNames
}

// QueryEventName sets a single EventName field in the Query objec
func QueryEventName(eventName event.Name) QueryField { return queryFieldEventNames{eventName} }

// QueryEmittedStartTime sets the EmittedStartTime field of the Query object
func QueryEmittedStartTime(emittedStartTime time.Time) QueryField {
	return queryFieldEmittedStartTime(emittedStartTime)
}
func (value queryFieldEmittedStartTime) queryFieldPointer(query *Query) interface{} {
	return &query.EmittedStartTime
}

// QueryEmittedEndTime sets the EmittedEndTime field of the Query object
func QueryEmittedEndTime(emittedEndTime time.Time) QueryField {
	return queryFieldEmittedEndTime(emittedEndTime)
}
func (value queryFieldEmittedEndTime) queryFieldPointer(query *Query) interface{} {
	return &query.EmittedEndTime
}

// QueryTestName sets the TestName field of the Query object
func QueryTestName(testName string) QueryField {
	return queryFieldTestName(testName)
}
func (value queryFieldTestName) queryFieldPointer(query *Query) interface{} { return &query.TestName }

// QueryTestStepLabel sets the TestStepLabel field of the Query object
func QueryTestStepLabel(testStepLabel string) QueryField {
	return queryFieldTestStepLabel(testStepLabel)
}
func (value queryFieldTestStepLabel) queryFieldPointer(query *Query) interface{} {
	return &query.TestStepLabel
}

// QueryRunID sets the RunID field of the Query object
func QueryRunID(runID types.RunID) QueryField {
	return func(eq *Query) {
		eq.RunID = runID
	}
}

// Emitter defines the interface that emitter objects must implement
type Emitter interface {
	Emit(event Data) error
}

// Fetcher defines the interface that fetcher objects must implement
type Fetcher interface {
	Fetch(fields ...QueryField) ([]Event, error)
}

// EmitterFetcher defines the interface that objects supporting emitting and fetching events must implement
type EmitterFetcher interface {
	Emitter
	Fetcher
}
