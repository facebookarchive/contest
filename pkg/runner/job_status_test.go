// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/stretchr/testify/require"
)

type dummyFrameworkEventManager struct {
	t *testing.T
}

func (fem dummyFrameworkEventManager) Emit(ctx xcontext.Context, event frameworkevent.Event) error {
	return nil
}
func (fem dummyFrameworkEventManager) Fetch(ctx xcontext.Context, fields ...frameworkevent.QueryField) ([]frameworkevent.Event, error) {
	require.Len(fem.t, fields, 2)
	require.Equal(fem.t, frameworkevent.QueryEventName(EventRunStarted), fields[0])
	require.Equal(fem.t, frameworkevent.QueryJobID(1), fields[1])
	return []frameworkevent.Event{
		{
			JobID:     1,
			EventName: EventRunStarted,
			Payload:   &[]json.RawMessage{json.RawMessage(`{"RunID":2}`)}[0],
			EmitTime:  time.Unix(2, 0),
		},
		{
			JobID:     1,
			EventName: EventRunStarted,
			Payload:   &[]json.RawMessage{json.RawMessage(`{"RunID":1}`)}[0],
			EmitTime:  time.Unix(1, 0),
		},
	}, nil
}

func (fem dummyFrameworkEventManager) FetchAsync(ctx xcontext.Context, fields ...frameworkevent.QueryField) ([]frameworkevent.Event, error) {
	require.Len(fem.t, fields, 2)
	require.Equal(fem.t, frameworkevent.QueryEventName(EventRunStarted), fields[0])
	require.Equal(fem.t, frameworkevent.QueryJobID(1), fields[1])
	return []frameworkevent.Event{
		{
			JobID:     1,
			EventName: EventRunStarted,
			Payload:   &[]json.RawMessage{json.RawMessage(`{"RunID":2}`)}[0],
			EmitTime:  time.Unix(2, 0),
		},
		{
			JobID:     1,
			EventName: EventRunStarted,
			Payload:   &[]json.RawMessage{json.RawMessage(`{"RunID":1}`)}[0],
			EmitTime:  time.Unix(1, 0),
		},
	}, nil
}

func TestBuildRunStatuses(t *testing.T) {
	jr := &JobRunner{
		targetMap:             nil,
		targetLock:            nil,
		frameworkEventManager: dummyFrameworkEventManager{t: t},
		testEvManager:         nil,
	}
	runStatuses, err := jr.BuildRunStatuses(&job.Job{
		ID:   1,
		Runs: 3,
	})
	require.NoError(t, err)
	require.Len(t, runStatuses, 2)
}
