// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package memory

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/stretchr/testify/require"
)

var (
	ctx = logrusctx.NewContext(logger.LevelDebug)
)

func TestMemory_GetTestEvents(t *testing.T) {
	stor, err := New()
	require.NoError(t, err)

	ev0 := testevent.Event{
		EmitTime: time.Now(),
		Header: &testevent.Header{
			JobID:         1,
			RunID:         2,
			TestName:      "3",
			TestStepLabel: "4",
		},
		Data: &testevent.Data{},
	}
	err = stor.StoreTestEvent(ctx, ev0)
	require.NoError(t, err)

	ev1 := testevent.Event{
		EmitTime: time.Now(),
		Header: &testevent.Header{
			JobID:         1,
			RunID:         5,
			TestName:      "3",
			TestStepLabel: "4",
		},
		Data: &testevent.Data{},
	}
	err = stor.StoreTestEvent(ctx, ev1)
	require.NoError(t, err)

	query, err := testevent.BuildQuery(
		testevent.QueryRunID(2),
		testevent.QueryTestName("3"),
	)
	require.NoError(t, err)

	evs, err := stor.GetTestEvents(ctx, query)
	require.NoError(t, err)

	require.Len(t, evs, 1)
	ev := evs[0]

	require.Equal(t, ev0, ev)
}
