// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package testevent

import (
	"encoding/json"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// Header models the header of a test event, which consists in metadatat hat defines the
// emitter of the events. The Header is under ConTest control and cannot be manipulated
// by the TestStep
type Header struct {
	JobID         types.JobID
	TestName      string
	TestStepLabel string
}

// Data models the data of a test event. It is populated by the TestStep
type Data struct {
	EventName     event.Name
	TestStepIndex uint
	Target        *target.Target
	Payload       *json.RawMessage
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
	TestName      string
	TestStepLabel string
}

// QueryField defines a function type used to set fields on a Query object
type QueryField func(opts *Query)

// QueryJobID sets the JobID field of the Query object
func QueryJobID(jobID types.JobID) QueryField {
	return func(eq *Query) {
		eq.JobID = jobID
	}
}

// QueryEventNames the EventNames field of the Query object
func QueryEventNames(eventNames []event.Name) QueryField {
	return func(eq *Query) {
		eq.EventNames = eventNames
	}
}

// QueryEventName sets a single EventName field in the Query object
func QueryEventName(eventName event.Name) QueryField {
	return func(eq *Query) {
		eventNames := []event.Name{eventName}
		eq.EventNames = eventNames
	}
}

// QueryEmittedStartTime sets the EmittedStartTime field of the Query object
func QueryEmittedStartTime(emittedStartTime time.Time) QueryField {
	return func(eq *Query) {
		eq.EmittedStartTime = emittedStartTime
	}
}

// QueryEmittedEndTime sets the EmittedEndTime field of the Query object
func QueryEmittedEndTime(emittedUntil time.Time) QueryField {
	return func(eq *Query) {
		eq.EmittedEndTime = emittedUntil
	}
}

// QueryTestName sets the TestName field of the Query object
func QueryTestName(testName string) QueryField {
	return func(eq *Query) {
		eq.TestName = testName
	}
}

// QueryTestStepLabel sets the TestStepLabel field of the Query object
func QueryTestStepLabel(testStepLabel string) QueryField {
	return func(eq *Query) {
		eq.TestStepLabel = testStepLabel
	}
}

// Emitter defines the interface that emitter objects must implement
type Emitter interface {
	Emit(event Data) error
}

// Fetcher defines the interface that fetcher objects must implement
type Fetcher interface {
	Fetch(fields []QueryField) ([]Event, error)
}

// EmitterFetcher defines the interface that objects supporting emitting and fetching events must implement
type EmitterFetcher interface {
	Emitter
	Fetcher
}
