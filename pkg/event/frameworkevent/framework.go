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
	// ApplyToQuery applied the field value to the query (into the appropriate field of the query)
	ApplyToQuery(query *Query)
}

// QueryFields is a set of field values for a Query object
type QueryFields []QueryField

// ToAbstract converts the slice to an abstract form so it could be validated
// using common rules for any QueryFields.
func (queryFields QueryFields) ToAbstract() (result event.QueryFields) {
	for _, queryField := range queryFields {
		result = append(result, queryField)
	}
	return
}

// Validate returns an error if a query cannot be formed using this queryFields.
func (queryFields QueryFields) Validate() error {
	return queryFields.ToAbstract().Validate()
}

// ApplyTo applies multiple QueryField-s to a query
func (queryFields QueryFields) ApplyTo(query *Query) error {
	if err := queryFields.Validate(); err != nil {
		return fmt.Errorf("query fields validation failed: %w", err)
	}

	for _, queryField := range queryFields {
		queryField.ApplyToQuery(query)
	}
	return nil
}

// BuildQuery compiles a Query from scratch using values of queryFields.
// It does basically just creates an empty query an applies queryFields to it.
func (queryFields QueryFields) BuildQuery() (*Query, error) {
	query := &Query{}
	if err := queryFields.ApplyTo(query); err != nil {
		return nil, fmt.Errorf("unable to apply query fields to a query, error: %w", err)
	}
	return query, nil
}

type queryFieldJobID types.JobID
type queryFieldEventNames []event.Name
type queryFieldEmittedStartTime time.Time
type queryFieldEmittedEndTime time.Time

func (value queryFieldJobID) ApplyToQuery(query *Query)      { query.JobID = types.JobID(value) }
func (value queryFieldEventNames) ApplyToQuery(query *Query) { query.EventNames = value }
func (value queryFieldEmittedStartTime) ApplyToQuery(query *Query) {
	query.EmittedStartTime = time.Time(value)
}
func (value queryFieldEmittedEndTime) ApplyToQuery(query *Query) {
	query.EmittedEndTime = time.Time(value)
}

// QueryJobID sets the JobID field of the Query object
func QueryJobID(jobID types.JobID) QueryField {
	return queryFieldJobID(jobID)
}

// QueryEventNames the EventNames field of the Query object
func QueryEventNames(eventNames []event.Name) QueryField {
	return queryFieldEventNames(eventNames)
}

// QueryEventName sets a single EventName field in the Query object
func QueryEventName(eventName event.Name) QueryField {
	return queryFieldEventNames{eventName}
}

// QueryEmittedStartTime sets the EmittedStartTime field of the Query object
func QueryEmittedStartTime(emittedStartTime time.Time) QueryField {
	return queryFieldEmittedStartTime(emittedStartTime)
}

// QueryEmittedEndTime sets the EmittedEndTime field of the Query object
func QueryEmittedEndTime(emittedUntil time.Time) QueryField {
	return queryFieldEmittedEndTime(emittedUntil)
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
