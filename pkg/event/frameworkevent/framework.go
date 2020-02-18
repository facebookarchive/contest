// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package frameworkevent

import (
	"encoding/json"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/types"
)

// Event represents an event emitted by the framework
type Event struct {
	JobID     types.JobID
	EventName event.Name
	Payload   *json.RawMessage
	EmitTime  time.Time
}

// New creates a new FrameworkEvent
func New() Event {
	return Event{}
}

// Query wraps information that are used to build event queries for framework events
type Query struct {
	event.Query
}

// QueryField defines a function type used to set fields on Query objects
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

// Emitter defines the interface that emitter objects for framework vents must implement
type Emitter interface {
	Emit(event Event) error
}

// Fetcher defines the interface that fetcher objects for framework events must implement
type Fetcher interface {
	Fetch(fields ...QueryField) ([]Event, error)
}

// EmitterFetcher defines the interface that objects supporting emitting and retrieving framework events must implement
type EmitterFetcher interface {
	Emitter
	Fetcher
}
