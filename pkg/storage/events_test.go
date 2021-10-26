// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"testing"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/types"
)

type testEventEmitterFixture struct {
	header          testevent.Header
	allowedEvents   []event.Name
	allowedMap      map[event.Name]bool
	forbiddenEvents []event.Name
	ctx             xcontext.Context
}

func mockTestEventEmitterData() *testEventEmitterFixture {
	allowedEvents := []event.Name{
		"TestEventAllowed1",
		"TestEventAllowed2",
	}

	return &testEventEmitterFixture{
		header: testevent.Header{
			JobID:         types.JobID(123),
			RunID:         types.RunID(456),
			TestName:      "TestStep",
			TestStepLabel: "TestLabel",
		},
		allowedEvents: allowedEvents,
		allowedMap: map[event.Name]bool{
			allowedEvents[0]: true,
			allowedEvents[1]: true,
		},
		forbiddenEvents: []event.Name{
			"TestEventForbidden1",
		},
		ctx: logrusctx.NewContext(logger.LevelDebug),
	}
}

func TestEmitUnrestricted(t *testing.T) {
	mockStorage(t)
	f := mockTestEventEmitterData()

	em := NewTestEventEmitter(f.header)
	require.NoError(t, em.Emit(f.ctx, testevent.Data{EventName: f.allowedEvents[0]}))
	require.NoError(t, em.Emit(f.ctx, testevent.Data{EventName: f.allowedEvents[1]}))
	require.NoError(t, em.Emit(f.ctx, testevent.Data{EventName: f.forbiddenEvents[0]}))
}

func TestEmitRestricted(t *testing.T) {
	mockStorage(t)
	f := mockTestEventEmitterData()

	em := NewTestEventEmitterWithAllowedEvents(f.header, &f.allowedMap)
	require.NoError(t, em.Emit(f.ctx, testevent.Data{EventName: f.allowedEvents[0]}))
	require.NoError(t, em.Emit(f.ctx, testevent.Data{EventName: f.allowedEvents[1]}))
	require.Error(t, em.Emit(f.ctx, testevent.Data{EventName: f.forbiddenEvents[0]}))
}

type testEventFetcherFixture struct {
	ctx xcontext.Context
}

func mockTestEventFetcherData() *testEventFetcherFixture {
	return &testEventFetcherFixture{
		ctx: logrusctx.NewContext(logger.LevelDebug),
	}
}

func TestTestEventFetcherConsistency(t *testing.T) {
	storage, storageAsync := mockStorage(t)
	f := mockTestEventFetcherData()
	ef := NewTestEventFetcher()

	// test with default context
	_, _ = ef.Fetch(f.ctx)
	require.Equal(t, storage.GetEventRequestCount(), 1)
	require.Equal(t, storageAsync.GetEventRequestCount(), 0)

	// test with explicit strong consistency
	ctx := WithConsistencyModel(f.ctx, ConsistentReadAfterWrite)
	_, _ = ef.Fetch(ctx)
	require.Equal(t, storage.GetEventRequestCount(), 2)
	require.Equal(t, storageAsync.GetEventRequestCount(), 0)

	// test with explicit relaxed consistency
	ctx = WithConsistencyModel(f.ctx, ConsistentEventually)
	_, _ = ef.Fetch(ctx)
	require.Equal(t, storage.GetEventRequestCount(), 2)
	require.Equal(t, storageAsync.GetEventRequestCount(), 1)
}

func TestFrameworkEventFetcherConsistency(t *testing.T) {
	storage, storageAsync := mockStorage(t)
	f := mockTestEventFetcherData()
	ef := NewFrameworkEventFetcher()

	// test with default context
	_, _ = ef.Fetch(f.ctx)
	require.Equal(t, storage.GetEventRequestCount(), 1)
	require.Equal(t, storageAsync.GetEventRequestCount(), 0)

	// test with explicit strong consistency
	ctx := WithConsistencyModel(f.ctx, ConsistentReadAfterWrite)
	_, _ = ef.Fetch(ctx)
	require.Equal(t, storage.GetEventRequestCount(), 2)
	require.Equal(t, storageAsync.GetEventRequestCount(), 0)

	// test with explicit relaxed consistency
	ctx = WithConsistencyModel(f.ctx, ConsistentEventually)
	_, _ = ef.Fetch(ctx)
	require.Equal(t, storage.GetEventRequestCount(), 2)
	require.Equal(t, storageAsync.GetEventRequestCount(), 1)
}
