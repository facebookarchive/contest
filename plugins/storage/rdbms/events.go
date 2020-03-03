// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rdbms

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

func assembleQuery(baseQuery bytes.Buffer, selectClauses []string) (string, error) {
	if len(selectClauses) == 0 {
		return "", fmt.Errorf("no select clauses available, the query should specify at least one clause")
	}
	initialClause := true
	for _, clause := range selectClauses {
		if initialClause {
			baseQuery.WriteString(fmt.Sprintf(" where %s", clause))
			initialClause = false
		} else {
			baseQuery.WriteString(fmt.Sprintf(" and %s", clause))
		}
	}
	baseQuery.WriteString(" order by event_id")
	return baseQuery.String(), nil
}

func buildEventQuery(baseQuery bytes.Buffer, eventQuery *event.Query) ([]string, []interface{}) {
	selectClauses := []string{}
	fields := []interface{}{}

	if eventQuery != nil && eventQuery.JobID != 0 {
		selectClauses = append(selectClauses, "job_id=?")
		fields = append(fields, eventQuery.JobID)
	}

	if eventQuery != nil && len(eventQuery.EventNames) != 0 {
		if len(eventQuery.EventNames) == 1 {
			selectClauses = append(selectClauses, "event_name=?")
		} else {
			queryStr := fmt.Sprintf("event_name in")
			for i := 0; i < len(eventQuery.EventNames); i++ {
				if i == 0 {
					queryStr = fmt.Sprintf("%s (?", queryStr)
				} else if i < len(eventQuery.EventNames)-1 {
					queryStr = fmt.Sprintf("%s, ?", queryStr)
				} else {
					queryStr = fmt.Sprintf("%s, ?)", queryStr)
				}
			}
			selectClauses = append(selectClauses, queryStr)
		}
		for i := 0; i < len(eventQuery.EventNames); i++ {
			fields = append(fields, eventQuery.EventNames[i])
		}
	}

	if eventQuery != nil && !eventQuery.EmittedStartTime.IsZero() {
		selectClauses = append(selectClauses, "emit_time>=?")
		fields = append(fields, eventQuery.EmittedStartTime)
	}
	if eventQuery != nil && !eventQuery.EmittedEndTime.IsZero() {
		selectClauses = append(selectClauses, "emit_time<=?")
		fields = append(fields, eventQuery.EmittedStartTime)
	}
	return selectClauses, fields
}

func buildFrameworkEventQuery(baseQuery bytes.Buffer, frameworkEventQuery *frameworkevent.Query) (string, []interface{}, error) {
	selectClauses, fields := buildEventQuery(baseQuery, &frameworkEventQuery.Query)
	query, err := assembleQuery(baseQuery, selectClauses)
	if err != nil {
		return "", nil, fmt.Errorf("could not assemble query for framework events: %v", err)

	}
	return query, fields, nil
}

func buildTestEventQuery(baseQuery bytes.Buffer, testEventQuery *testevent.Query) (string, []interface{}, error) {

	selectClauses, fields := buildEventQuery(baseQuery, &testEventQuery.Query)

	if testEventQuery != nil && testEventQuery.RunID != types.RunID(0) {
		selectClauses = append(selectClauses, "run_id=?")
		fields = append(fields, testEventQuery.RunID)
	}

	if testEventQuery != nil && testEventQuery.TestName != "" {
		selectClauses = append(selectClauses, "test_name=?")
		fields = append(fields, testEventQuery.TestName)
	}
	if testEventQuery != nil && testEventQuery.TestStepLabel != "" {
		selectClauses = append(selectClauses, "test_step_label=?")
		fields = append(fields, testEventQuery.TestStepLabel)
	}
	query, err := assembleQuery(baseQuery, selectClauses)
	if err != nil {
		return "", nil, fmt.Errorf("could not assemble query for framework events: %v", err)

	}
	return query, fields, nil
}

// TestEventField is a function type which retrieves information from a TestEvent object.
type TestEventField func(ev testevent.Event) interface{}

// TestEventJobID returns the JobID from an events.TestEvent object
func TestEventJobID(ev testevent.Event) interface{} {
	if ev.Header == nil {
		return nil
	}
	return ev.Header.JobID
}

// TestEventRunID returns the RunID from a
func TestEventRunID(ev testevent.Event) interface{} {
	if ev.Header == nil {
		return nil
	}
	return ev.Header.RunID
}

// TestEventTestName returns the test name from an events.TestEvent object
func TestEventTestName(ev testevent.Event) interface{} {
	if ev.Header == nil {
		return nil
	}
	return ev.Header.TestName
}

// TestEventTestStepLabel returns the test step label from an events.TestEvent object
func TestEventTestStepLabel(ev testevent.Event) interface{} {
	if ev.Header == nil {
		return nil
	}
	return ev.Header.TestStepLabel
}

// TestEventName returns the event name from an events.TestEvent object
func TestEventName(ev testevent.Event) interface{} {
	if ev.Data == nil {
		return nil
	}
	return ev.Data.EventName
}

// TestEventTargetName returns the target name from an events.TestEvent object
func TestEventTargetName(ev testevent.Event) interface{} {
	if ev.Data == nil || ev.Data.Target == nil {
		return nil
	}
	return ev.Data.Target.Name
}

// TestEventTargetID returns the target id from an events.TestEvent object
func TestEventTargetID(ev testevent.Event) interface{} {
	if ev.Data == nil || ev.Data.Target == nil {
		return nil
	}
	return ev.Data.Target.ID
}

// TestEventPayload returns the payload from an events.TestEvent object
func TestEventPayload(ev testevent.Event) interface{} {
	if ev.Data == nil {
		return nil
	}
	return ev.Data.Payload
}

// TestEventEmitTime returns the emission timestamp from an events.TestEvent object
func TestEventEmitTime(ev testevent.Event) interface{} {
	return ev.EmitTime
}

// StoreTestEvent appends an event to the internal buffer and triggers a flush
// when the internal storage utilization goes beyond `testEventsFlushSize`
func (r *RDBMS) StoreTestEvent(event testevent.Event) error {

	if err := r.init(); err != nil {
		return fmt.Errorf("could not initialize database: %v", err)
	}

	var doFlush bool

	r.testEventsLock.Lock()
	r.buffTestEvents = append(r.buffTestEvents, event)
	if len(r.buffTestEvents) == r.testEventsFlushSize {
		doFlush = true
	}
	r.testEventsLock.Unlock()
	if doFlush {
		return r.FlushTestEvents()
	}
	return nil
}

// FlushTestEvents forces a flush of the pending test events to the database
func (r *RDBMS) FlushTestEvents() error {
	r.testEventsLock.Lock()
	defer r.testEventsLock.Unlock()

	if len(r.buffTestEvents) == 0 {
		return nil
	}

	insertStatement := "insert into test_events (job_id, run_id, test_name, test_step_label, event_name, target_name, target_id, payload, emit_time) values (?, ?, ?, ?, ?, ?, ?, ?, ?)"
	for _, event := range r.buffTestEvents {
		_, err := r.db.Exec(
			insertStatement,
			TestEventJobID(event),
			TestEventRunID(event),
			TestEventTestName(event),
			TestEventTestStepLabel(event),
			TestEventName(event),
			TestEventTargetName(event),
			TestEventTargetID(event),
			TestEventPayload(event),
			TestEventEmitTime(event))
		if err != nil {
			return fmt.Errorf("could not store event in database: %v", err)
		}
	}
	r.buffTestEvents = nil
	return nil
}

// GetTestEvents retrieves test events matching the query fields provided
func (r *RDBMS) GetTestEvents(eventQuery *testevent.Query) ([]testevent.Event, error) {

	if err := r.init(); err != nil {
		return nil, fmt.Errorf("could not initialize database: %v", err)
	}

	// Flush pending events before Get operations
	err := r.FlushTestEvents()
	if err != nil {
		return nil, fmt.Errorf("could not flush events before reading events: %v", err)
	}

	r.testEventsLock.Lock()
	defer r.testEventsLock.Unlock()

	baseQuery := bytes.Buffer{}
	baseQuery.WriteString("select event_id, job_id, run_id, test_name, test_step_label, event_name, target_name, target_id, payload, emit_time from test_events")
	query, fields, err := buildTestEventQuery(baseQuery, eventQuery)
	if err != nil {
		return nil, fmt.Errorf("could not execute select query for test events: %v", err)
	}

	results := []testevent.Event{}
	log.Debugf("Executing query: %s", query)
	rows, err := r.db.Query(query, fields...)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := rows.Close(); err != nil {
			log.Warningf("Failed to close rows from Query statement: %v", err)
		}
	}()

	// TargetName and TargetID might be null, so a type which supports null should be used with Scan
	var (
		targetName sql.NullString
		targetID   sql.NullString
		payload    sql.NullString
	)

	for rows.Next() {
		data := testevent.Data{}
		header := testevent.Header{}
		event := testevent.New(&header, &data)

		var eventID int
		err := rows.Scan(
			&eventID,
			&header.JobID,
			&header.RunID,
			&header.TestName,
			&header.TestStepLabel,
			&data.EventName,
			&targetName,
			&targetID,
			&payload,
			&event.EmitTime,
		)
		if err != nil {
			return nil, fmt.Errorf("could not read results from db: %v", err)
		}
		if targetName.Valid || targetID.Valid {
			t := target.Target{Name: targetName.String, ID: targetID.String}
			data.Target = &t
		}

		if payload.Valid {
			rawPayload := json.RawMessage(payload.String)
			data.Payload = &rawPayload

		}

		results = append(results, event)
	}
	return results, nil
}

// FrameworkEventField is a function type which retrieves information from a FrameworkEvent object
type FrameworkEventField func(ev frameworkevent.Event) interface{}

// FrameworkEventJobID returns the JobID from a events.TestEvent object
func FrameworkEventJobID(ev frameworkevent.Event) interface{} {
	return ev.JobID
}

// FrameworkEventName returns the name for the FrameworkEvent object
func FrameworkEventName(ev frameworkevent.Event) interface{} {
	return ev.EventName
}

// FrameworkEventPayload returns the payload from a events.FrameworkEvent object
func FrameworkEventPayload(ev frameworkevent.Event) interface{} {
	return ev.Payload
}

// FrameworkEventEmitTime returns the emission timestamp from a events.FrameworkEvent object
func FrameworkEventEmitTime(ev frameworkevent.Event) interface{} {
	return ev.EmitTime
}

// StoreFrameworkEvent appends an event to the internal buffer and triggers a flush
// when the internal storage utilization goes beyond `frameworkEventsFlushSize`
func (r *RDBMS) StoreFrameworkEvent(event frameworkevent.Event) error {

	if err := r.init(); err != nil {
		return fmt.Errorf("could not initialize database: %v", err)
	}

	var doFlush bool

	r.frameworkEventsLock.Lock()
	r.buffFrameworkEvents = append(r.buffFrameworkEvents, event)
	if len(r.buffFrameworkEvents) >= r.frameworkEventsFlushSize {
		doFlush = true
	}
	r.frameworkEventsLock.Unlock()
	if doFlush {
		return r.FlushFrameworkEvents()
	}
	return nil
}

// FlushFrameworkEvents forces a flush of the pending frameworks events to the database
func (r *RDBMS) FlushFrameworkEvents() error {
	r.frameworkEventsLock.Lock()
	defer r.frameworkEventsLock.Unlock()

	if len(r.buffFrameworkEvents) == 0 {
		return nil
	}

	insertStatement := "insert into framework_events (job_id, event_name, payload, emit_time) values (?, ?, ?, ?)"
	for _, event := range r.buffFrameworkEvents {
		_, err := r.db.Exec(
			insertStatement,
			FrameworkEventJobID(event),
			FrameworkEventName(event),
			FrameworkEventPayload(event),
			FrameworkEventEmitTime(event))
		if err != nil {
			return fmt.Errorf("could not store event in database: %v", err)
		}
	}
	r.buffFrameworkEvents = nil
	return nil
}

// GetFrameworkEvent retrieves framework events matching the query fields provided
func (r *RDBMS) GetFrameworkEvent(eventQuery *frameworkevent.Query) ([]frameworkevent.Event, error) {

	if err := r.init(); err != nil {
		return nil, fmt.Errorf("could not initialize database: %v", err)
	}

	// Flush pending events before Get operations
	err := r.FlushFrameworkEvents()
	if err != nil {
		return nil, fmt.Errorf("could not flush events before reading events: %v", err)
	}

	baseQuery := bytes.Buffer{}
	baseQuery.WriteString(`select event_id, job_id, event_name, payload, emit_time from framework_events`)
	query, fields, err := buildFrameworkEventQuery(baseQuery, eventQuery)
	if err != nil {
		return nil, fmt.Errorf("could not execute select query for test events: %v", err)
	}

	r.frameworkEventsLock.Lock()
	defer r.frameworkEventsLock.Unlock()

	results := []frameworkevent.Event{}
	log.Debugf("Executing query: %s", query)
	rows, err := r.db.Query(query, fields...)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := rows.Close(); err != nil {
			log.Warningf("Failed to close rows from Query statement: %v", err)
		}
	}()

	for rows.Next() {
		event := frameworkevent.New()
		var eventID int
		err := rows.Scan(&eventID, &event.JobID, &event.EventName, &event.Payload, &event.EmitTime)
		if err != nil {
			return nil, fmt.Errorf("could not read results from db: %v", err)
		}
		results = append(results, event)
	}
	return results, nil
}
