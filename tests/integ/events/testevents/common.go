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
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/targetmanager"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/tests/integ/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func mustBuildQuery(t require.TestingT, queryFields ...testevent.QueryField) *testevent.Query {
	eventQuery, err := testevent.BuildQuery(queryFields...)
	require.NoError(t, err)
	return eventQuery
}

func populateTestEvents(backend storage.Storage, emitTime time.Time) error {
	data := []byte("{ 'test_key': 'test_value' }")
	payload := (*json.RawMessage)(&data)

	testTargetFirst := target.Target{Name: "ATargetName", ID: "ATargetID", FQDN: "AFQDN"}
	testTargetSecond := target.Target{Name: "BTargetName", ID: "BTargetID", FQDN: "BFQDN"}

	hdrFirst := testevent.Header{JobID: 1, TestName: "ATestName", TestStepLabel: "TestStepLabel"}
	dataFirst := testevent.Data{EventName: event.Name("AEventName"), Target: &testTargetFirst, Payload: payload}

	hdrSecond := testevent.Header{JobID: 2, TestName: "BTestName", TestStepLabel: "TestStepLabel"}
	dataSecond := testevent.Data{EventName: event.Name("BEventName"), Target: &testTargetSecond, Payload: payload}

	hdrThird := testevent.Header{JobID: 2, TestName: "CTestName", TestStepLabel: ""}
	dataThird := testevent.Data{EventName: targetmanager.EventName, Payload: payload}

	eventFirst := testevent.Event{Header: &hdrFirst, Data: &dataFirst, EmitTime: emitTime}
	eventSecond := testevent.Event{Header: &hdrSecond, Data: &dataSecond, EmitTime: emitTime}
	eventThird := testevent.Event{Header: &hdrThird, Data: &dataThird, EmitTime: emitTime}

	if err := backend.StoreTestEvent(eventFirst); err != nil {
		return err
	}
	if err := backend.StoreTestEvent(eventSecond); err != nil {
		return err
	}
	if err := backend.StoreTestEvent(eventThird); err != nil {
		return err
	}
	return nil
}

func assertTestEvents(t *testing.T, ev []testevent.Event, emitTime time.Time) {

	data := []byte("{ 'test_key': 'test_value' }")
	payload := (*json.RawMessage)(&data)

	assert.Equal(t, types.JobID(1), ev[0].Header.JobID)
	assert.Equal(t, "ATestName", ev[0].Header.TestName)
	assert.Equal(t, "TestStepLabel", ev[0].Header.TestStepLabel)
	assert.Equal(t, event.Name("AEventName"), ev[0].Data.EventName)
	assert.Equal(t, "ATargetName", ev[0].Data.Target.Name)
	assert.Equal(t, "ATargetID", ev[0].Data.Target.ID)
	assert.Equal(t, payload, ev[0].Data.Payload)
	assert.Equal(t, emitTime.UTC(), ev[0].EmitTime.UTC())

	if len(ev) == 2 {
		assert.Equal(t, types.JobID(2), ev[1].Header.JobID)
		assert.Equal(t, "BTestName", ev[1].Header.TestName)
		assert.Equal(t, "TestStepLabel", ev[1].Header.TestStepLabel)
		assert.Equal(t, event.Name("BEventName"), ev[1].Data.EventName)
		assert.Equal(t, "BTargetName", ev[1].Data.Target.Name)
		assert.Equal(t, "BTargetID", ev[1].Data.Target.ID)
		assert.Equal(t, payload, ev[1].Data.Payload)
		assert.Equal(t, emitTime.UTC(), ev[1].EmitTime.UTC())
	}
}

type TestEventsSuite struct {
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

func (suite *TestEventsSuite) SetupTest() {
	suite.txStorage = common.InitStorage(suite.storage)
}

func (suite *TestEventsSuite) TearDownTest() {
	common.FinalizeStorage(suite.txStorage)
}

func (suite *TestEventsSuite) TestRetrieveSingleTestEvent() {

	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	testEventQuery := mustBuildQuery(suite.T(), testevent.QueryTestName("ATestName"))
	results, err := suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(results))
	assertTestEvents(suite.T(), []testevent.Event{results[0]}, emitTime)

}

func (suite *TestEventsSuite) TestRetrieveMultipleTestEvents() {

	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	testEventQuery := mustBuildQuery(suite.T(), testevent.QueryTestStepLabel("TestStepLabel"))
	results, err := suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(results))
	assertTestEvents(suite.T(), results, emitTime)
}

func (suite *TestEventsSuite) TestRetrievesSingleTestEventByEmitTime() {

	delta := 10 * time.Second
	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	emitTime = emitTime.Add(delta)
	err = populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	testEventQuery := mustBuildQuery(suite.T(), testevent.QueryEmittedStartTime(emitTime))
	results, err := suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(results))
	assertTestEvents(suite.T(), results, emitTime)

	testEventQuery = mustBuildQuery(suite.T(), testevent.QueryEmittedStartTime(emitTime.Add(-delta)))
	results, err = suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 6, len(results))
	err = populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

}

func (suite *TestEventsSuite) TestRetrievesMultipleTestEventsByName() {

	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	eventNames := []event.Name{event.Name("AEventName"), event.Name("BEventName")}
	testEventQuery := mustBuildQuery(suite.T(), testevent.QueryEventNames(eventNames))
	results, err := suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(results))
	assertTestEvents(suite.T(), results, emitTime)
}

func (suite *TestEventsSuite) TestRetrieveSingleTestEventsByName() {

	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	testEventQuery := mustBuildQuery(suite.T(), testevent.QueryEventName(event.Name("AEventName")))
	results, err := suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(results))
	assertTestEvents(suite.T(), results, emitTime)
}

func (suite *TestEventsSuite) TestRetrieveSingleTestEventsByNameAndJobID() {

	emitTime := time.Now().Truncate(2 * time.Second)
	err := populateTestEvents(suite.txStorage, emitTime)
	require.NoError(suite.T(), err)

	testEventQuery := mustBuildQuery(suite.T(),
		testevent.QueryEventName(event.Name("AEventName")),
		testevent.QueryJobID(1),
	)
	results, err := suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(results))
	assertTestEvents(suite.T(), results, emitTime)
}

type dummyTestEventEmitter struct {}

func (emitter dummyTestEventEmitter) Emit(event testevent.Data) error {
	return nil
}

func (suite *TestEventsSuite) TestTargetManagerEvents() {
	emitter := targetmanager.EventEmitter{
		TestEventEmitter: dummyTestEventEmitter{},
	}
	err := emitter.Emit(testevent.Data{EventName: "not-a-target-manager"})
	require.Error(suite.T(), err)

	err = emitter.Emit(testevent.Data{EventName: targetmanager.EventName})
	require.NoError(suite.T(), err)

	err = populateTestEvents(suite.txStorage, time.Now())
	require.NoError(suite.T(), err)

	testEventQuery := mustBuildQuery(suite.T(),
		testevent.QueryEventName(targetmanager.EventName),
	)
	results, err := suite.txStorage.GetTestEvents(testEventQuery)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(results))
	assert.Equal(suite.T(), targetmanager.EventName, results[0].Data.EventName)
}
