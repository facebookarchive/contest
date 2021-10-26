// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package noop

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/stretchr/testify/require"
)

var (
	ctx = logrusctx.NewContext(logger.LevelDebug)
)

func TestNoopNew(t *testing.T) {
	tl := New()
	require.NotNil(t, tl)
	require.IsType(t, &Noop{}, tl)
}

func TestNoopLock(t *testing.T) {
	tl := New()
	// we don't enforce that at least one target is passed, as checking on
	// non-zero targets is the framework's responsibility, not the plugin.
	// So, zero targets is OK.
	jobID := types.JobID(123)
	jobTargetManagerAcquireTimeout := 5 * time.Minute
	require.Nil(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, nil))
	require.Nil(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, []*target.Target{}))
	require.Nil(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, []*target.Target{
		&target.Target{ID: "blah"},
	}))
	require.Nil(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, []*target.Target{
		&target.Target{ID: "blah"},
		&target.Target{ID: "bleh"},
	}))
}

func TestNoopTryLock(t *testing.T) {
	tl := New()
	// we don't enforce that at least one target is passed, as checking on
	// non-zero targets is the framework's responsibility, not the plugin.
	// So, zero targets is OK.
	jobID := types.JobID(123)
	jobTargetManagerAcquireTimeout := 5 * time.Minute
	_, err := tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, nil, 0)
	require.Nil(t, err)
	_, err = tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, []*target.Target{}, 0)
	require.Nil(t, err)
	_, err = tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, []*target.Target{
		&target.Target{ID: "blah"},
	}, 1)
	require.Nil(t, err)
	_, err = tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, []*target.Target{
		&target.Target{ID: "blah"},
		&target.Target{ID: "bleh"},
	}, 2)
	require.Nil(t, err)
}

func TestNoopUnlock(t *testing.T) {
	tl := New()
	// we don't enforce that at least one target is passed, as checking on
	// non-zero targets is the framework's responsibility, not the plugin.
	// So, zero targets is OK.
	jobID := types.JobID(123)
	require.Nil(t, tl.Unlock(ctx, jobID, nil))
	require.Nil(t, tl.Unlock(ctx, jobID, []*target.Target{}))
	require.Nil(t, tl.Unlock(ctx, jobID, []*target.Target{
		&target.Target{ID: "blah"},
	}))
	require.Nil(t, tl.Unlock(ctx, jobID, []*target.Target{
		&target.Target{ID: "blah"},
		&target.Target{ID: "bleh"},
	}))
}
