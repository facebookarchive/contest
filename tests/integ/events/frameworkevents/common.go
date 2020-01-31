// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	FrameworkEventsFlushInterval     = 10 * time.Second
	FrameworkEventsFlushSize     int = 0
)

func populateFrameworkEvents(backend storage.Storage, emitTime time.Time) error {

	data := []byte("{ 'test_key': 'test_value' }")
	payload := (*json.RawMessage)(&data)

	eventFirst := frameworkevent.Event{JobID: 1, EventName: "AFrameworkEvent", Payload: payload, EmitTime: emitTime}
	eventSecond := frameworkevent.Event{JobID: 1, EventName: "BFrameworkEvent", Payload: payload, EmitTime: emitTime}

	err := backend.StoreFrameworkEvent(eventFirst)
	if err != nil {
		return err
	}
	return backend.StoreFrameworkEvent(eventSecond)
}

func assertFrameworkEvents(t *testing.T, ev []frameworkevent.Event, emitTime time.Time) {

	data := []byte("{ 'test_key': 'test_value' }")
	payload := (*json.RawMessage)(&data)

	assert.Equal(t, types.JobID(1), ev[0].JobID)
	assert.Equal(t, event.Name("AFrameworkEvent"), ev[0].EventName)
	assert.Equal(t, payload, ev[0].Payload)
	assert.Equal(t, emitTime.UTC(), ev[0].EmitTime.UTC())

	if len(ev) == 2 {
		assert.Equal(t, types.JobID(1), ev[1].JobID)
		assert.Equal(t, event.Name("BFrameworkEvent"), ev[1].EventName)
		assert.Equal(t, payload, ev[1].Payload)
		assert.Equal(t, emitTime.UTC(), ev[1].EmitTime.UTC())
	}
}

type FrameworkEventsSuite struct {
	suite.Suite
	storage storage.Storage
}

func (suite *FrameworkEventsSuite) TearDownTest() {
	suite.storage.Reset()
}

func (suite *FrameworkEventsSuite) TestPersistFrameworkEvents() {
	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateFrameworkEvents(suite.storage, emitTime)
	require.NoError(suite.T(), err)

	eventQuery := &frameworkevent.Query{}
	frameworkevent.QueryEventName("AFrameworkEvent")(eventQuery)
}

func (suite *FrameworkEventsSuite) TestRetrieveSingleFrameworkEvent() {

	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateFrameworkEvents(suite.storage, emitTime)
	require.NoError(suite.T(), err)

	eventQuery := &frameworkevent.Query{}
	frameworkevent.QueryEventName("AFrameworkEvent")(eventQuery)
	results, err := suite.storage.GetFrameworkEvent(eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(results))
	assertFrameworkEvents(suite.T(), []frameworkevent.Event{results[0]}, emitTime)
}

func (suite *FrameworkEventsSuite) TestRetrieveMultipleFrameworkEvents() {

	eventQuery := &frameworkevent.Query{}
	frameworkevent.QueryJobID(types.JobID(1))(eventQuery)
	results, err := suite.storage.GetFrameworkEvent(eventQuery)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(results))

	emitTime := time.Now().Truncate(2 * time.Second)
	err = populateFrameworkEvents(suite.storage, emitTime)
	require.NoError(suite.T(), err)

	eventQuery = &frameworkevent.Query{}
	frameworkevent.QueryJobID(types.JobID(1))(eventQuery)
	results, err = suite.storage.GetFrameworkEvent(eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(results))
	assertFrameworkEvents(suite.T(), results, emitTime)
}

func (suite *FrameworkEventsSuite) TestRetrieveSingleFrameworkEventByEmitTime() {

	delta := 10 * time.Second
	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateFrameworkEvents(suite.storage, emitTime)
	require.NoError(suite.T(), err)

	emitTime = emitTime.Add(delta)
	err = populateFrameworkEvents(suite.storage, emitTime)
	require.NoError(suite.T(), err)

	eventQuery := &frameworkevent.Query{}
	frameworkevent.QueryEmittedStartTime(emitTime)(eventQuery)
	results, err := suite.storage.GetFrameworkEvent(eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(results))
	assertFrameworkEvents(suite.T(), results, emitTime)

	eventQuery = &frameworkevent.Query{}
	frameworkevent.QueryEmittedStartTime(emitTime.Add(-delta))(eventQuery)
	results, err = suite.storage.GetFrameworkEvent(eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 4, len(results))
}
