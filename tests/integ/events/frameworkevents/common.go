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
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/tests/integ/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	ctx = logrusctx.NewContext(logger.LevelDebug)
)

func mustBuildQuery(t require.TestingT, queryFields ...frameworkevent.QueryField) *frameworkevent.Query {
	eventQuery, err := frameworkevent.BuildQuery(queryFields...)
	require.NoError(t, err)
	return eventQuery
}

func populateFrameworkEvents(backend storage.Storage, emitTime time.Time) error {

	data := []byte("{ 'test_key': 'test_value' }")
	payload := (*json.RawMessage)(&data)

	eventFirst := frameworkevent.Event{JobID: 1, EventName: "AFrameworkEvent", Payload: payload, EmitTime: emitTime}
	eventSecond := frameworkevent.Event{JobID: 1, EventName: "BFrameworkEvent", Payload: payload, EmitTime: emitTime}

	err := backend.StoreFrameworkEvent(ctx, eventFirst)
	if err != nil {
		return err
	}
	return backend.StoreFrameworkEvent(ctx, eventSecond)
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

	// storage is the storage engine initially configured by the upper level TestSuite,
	// which either configures a memory or a rdbms storage backend.
	storage storage.Storage

	// txStorage storage is initialized from storage at the beginning of each test. If
	// the backend supports transactions, txStorage runs within a transaction. At the end
	// of the job txStorage is finalized: it's either committed or rolled back, depending
	// what the backend supports
	txStorage storage.Storage
}

func (suite *FrameworkEventsSuite) SetupTest() {
	suite.txStorage = common.InitStorage(suite.storage)
}

func (suite *FrameworkEventsSuite) TearDownTest() {
	common.FinalizeStorage(suite.txStorage)
}

func (suite *FrameworkEventsSuite) TestPersistFrameworkEvents() {
	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateFrameworkEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	mustBuildQuery(suite.T(), frameworkevent.QueryEventName("AFrameworkEvent"))
}

func (suite *FrameworkEventsSuite) TestRetrieveSingleFrameworkEvent() {

	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateFrameworkEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	eventQuery := mustBuildQuery(suite.T(), frameworkevent.QueryEventName("AFrameworkEvent"))
	results, err := suite.txStorage.GetFrameworkEvent(ctx, eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(results))
	assertFrameworkEvents(suite.T(), []frameworkevent.Event{results[0]}, emitTime)
}

func (suite *FrameworkEventsSuite) TestRetrieveMultipleFrameworkEvents() {

	eventQuery := mustBuildQuery(suite.T(), frameworkevent.QueryJobID(1))
	results, err := suite.txStorage.GetFrameworkEvent(ctx, eventQuery)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(results))

	emitTime := time.Now().Truncate(2 * time.Second)
	err = populateFrameworkEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	eventQuery = mustBuildQuery(suite.T(), frameworkevent.QueryJobID(1))
	results, err = suite.txStorage.GetFrameworkEvent(ctx, eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(results))
	assertFrameworkEvents(suite.T(), results, emitTime)
}

func (suite *FrameworkEventsSuite) TestRetrieveSingleFrameworkEventByEmitTime() {

	delta := 10 * time.Second
	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateFrameworkEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	emitTime = emitTime.Add(delta)
	err = populateFrameworkEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	eventQuery := mustBuildQuery(suite.T(), frameworkevent.QueryEmittedStartTime(emitTime))
	results, err := suite.txStorage.GetFrameworkEvent(ctx, eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(results))
	assertFrameworkEvents(suite.T(), results, emitTime)

	eventQuery = mustBuildQuery(suite.T(), frameworkevent.QueryEmittedStartTime(emitTime.Add(-delta)))
	results, err = suite.txStorage.GetFrameworkEvent(ctx, eventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 4, len(results))
}
