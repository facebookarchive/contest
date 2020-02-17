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
type queryFieldTestName string
type queryFieldTestStepLabel string

func (value queryFieldJobID) ApplyToQuery(query *Query)      { query.JobID = types.JobID(value) }
func (value queryFieldEventNames) ApplyToQuery(query *Query) { query.EventNames = value }
func (value queryFieldEmittedStartTime) ApplyToQuery(query *Query) {
	query.EmittedStartTime = time.Time(value)
}
func (value queryFieldEmittedEndTime) ApplyToQuery(query *Query) {
	query.EmittedEndTime = time.Time(value)
}
func (value queryFieldTestName) ApplyToQuery(query *Query)      { query.TestName = string(value) }
func (value queryFieldTestStepLabel) ApplyToQuery(query *Query) { query.TestStepLabel = string(value) }

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

// QueryTestName sets the TestName field of the Query object
func QueryTestName(testName string) QueryField {
	return queryFieldTestName(testName)
}

// QueryTestStepLabel sets the TestStepLabel field of the Query object
func QueryTestStepLabel(testStepLabel string) QueryField {
	return queryFieldTestStepLabel(testStepLabel)
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
