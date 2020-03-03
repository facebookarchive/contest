// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package frameworkevent

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/internal/querytools"
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
	for _, queryField := range queryFields {
		if err := querytools.ApplyQueryField(queryField.queryFieldPointer(query), queryField); err != nil {
			return nil, fmt.Errorf("unable to apply field %T(%v): %w", queryField, queryField, err)
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
