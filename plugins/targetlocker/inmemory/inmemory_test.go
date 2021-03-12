// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package inmemory

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	jobID                                 = types.JobID(123)
	otherJobID                            = types.JobID(456)
	defaultJobTargetManagerAcquireTimeout = 5 * time.Minute

	targetOne  = target.Target{ID: "001"}
	targetTwo  = target.Target{ID: "002"}
	oneTarget  = []*target.Target{&targetOne}
	twoTargets = []*target.Target{&targetOne, &targetTwo}

	ctx = logrusctx.NewContext(logger.LevelDebug)
)

func TestInMemoryNew(t *testing.T) {
	tl := New()
	require.NotNil(t, tl)
	require.IsType(t, &InMemory{}, tl)
}

func TestInMemoryLockInvalidJobIDAndNoTargets(t *testing.T) {
	tl := New()
	assert.Error(t, tl.Lock(ctx, 0, defaultJobTargetManagerAcquireTimeout, nil))
}

func TestInMemoryLockValidJobIDAndNoTargets(t *testing.T) {
	tl := New()
	assert.Error(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, nil))
}

func TestInMemoryLockInvalidJobIDAndOneTarget(t *testing.T) {
	tl := New()
	assert.Error(t, tl.Lock(ctx, 0, defaultJobTargetManagerAcquireTimeout, oneTarget))
}

func TestInMemoryLockValidJobIDAndOneTarget(t *testing.T) {
	tl := New()
	require.NoError(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, oneTarget))
}

func TestInMemoryLockValidJobIDAndTwoTargets(t *testing.T) {
	tl := New()
	require.NoError(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, twoTargets))
}

func TestInMemoryLockReentrantLock(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	require.NoError(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets))
	require.NoError(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets))
}

func TestInMemoryLockReentrantLockDifferentJobID(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	require.NoError(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets))
	require.Error(t, tl.Lock(ctx, jobID+1, jobTargetManagerAcquireTimeout, twoTargets))
}

func TestInMemoryTryLockOne(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	res, err := tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, oneTarget, 1)
	require.NoError(t, err)
	require.Equal(t, oneTarget[0].ID, res[0])
}

func TestInMemoryTryLockTwo(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	res, err := tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets, 2)
	require.NoError(t, err)
	// order is not guaranteed
	require.Contains(t, res, twoTargets[0].ID)
	require.Contains(t, res, twoTargets[1].ID)
}

func TestInMemoryTryLockZeroLimited(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	// only request one
	res, err := tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets, 0)
	require.NoError(t, err)
	// it is allowed to set the limit to zero
	require.Equal(t, len(res), 0)
}

func TestInMemoryTryLockTwoHigherLimit(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	// limit is just an upper bound, can be higher
	res, err := tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets, 100)
	require.NoError(t, err)
	// order is not guaranteed
	require.Contains(t, res, twoTargets[0].ID)
	require.Contains(t, res, twoTargets[1].ID)
}

func TestInMemoryTryLockOneLimited(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	// only request one
	res, err := tl.TryLock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets, 1)
	require.NoError(t, err)
	require.Equal(t, len(res), 1)
	// API doesn't require it, but locker guarantees order
	// so the first one should have been locked,
	// the second not because limit was 1
	require.Contains(t, res, twoTargets[0].ID)
	require.NotContains(t, res, twoTargets[1].ID)
}

func TestInMemoryTryLockOneOfTwo(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	require.NoError(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, oneTarget))
	// now tryLock both with other ID
	res, err := tl.TryLock(ctx, jobID+1, jobTargetManagerAcquireTimeout, twoTargets, 2)
	require.NoError(t, err)
	// should have locked 1 but not 0
	require.NotContains(t, res, twoTargets[0].ID)
	require.Contains(t, res, twoTargets[1].ID)
}

func TestInMemoryTryLockNoneOfTwo(t *testing.T) {
	tl := New()
	jobTargetManagerAcquireTimeout := 10 * time.Second
	require.NoError(t, tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, twoTargets))
	// now tryLock both with other ID
	res, err := tl.TryLock(ctx, jobID+1, jobTargetManagerAcquireTimeout, twoTargets, 2)
	// should have locked zero targets, but no error
	require.NoError(t, err)
	require.Empty(t, res)
}

func TestInMemoryUnlockInvalidJobIDAndNoTargets(t *testing.T) {
	tl := New()
	assert.Error(t, tl.Unlock(ctx, jobID, nil))
}

func TestInMemoryUnlockValidJobIDAndNoTargets(t *testing.T) {
	tl := New()
	assert.Error(t, tl.Unlock(ctx, jobID, nil))
}

func TestInMemoryUnlockInvalidJobIDAndOneTarget(t *testing.T) {
	tl := New()
	assert.Error(t, tl.Unlock(ctx, 0, oneTarget))
}

func TestInMemoryUnlockValidJobIDAndOneTarget(t *testing.T) {
	tl := New()
	require.Error(t, tl.Unlock(ctx, jobID, oneTarget))
}

func TestInMemoryUnlockValidJobIDAndTwoTargets(t *testing.T) {
	tl := New()
	require.Error(t, tl.Unlock(ctx, jobID, twoTargets))
}

func TestInMemoryUnlockUnlockTwice(t *testing.T) {
	tl := New()
	err := tl.Unlock(ctx, jobID, oneTarget)
	ctx.Logger().Debugf("%v", err)
	assert.Error(t, err)
	assert.Error(t, tl.Unlock(ctx, jobID, oneTarget))
}

func TestInMemoryUnlockReentrantLockDifferentJobID(t *testing.T) {
	tl := New()
	require.Error(t, tl.Unlock(ctx, jobID, twoTargets))
	assert.Error(t, tl.Unlock(ctx, jobID+1, twoTargets))
}

func TestInMemoryLockUnlockSameJobID(t *testing.T) {
	tl := New()
	require.NoError(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, twoTargets))
	assert.NoError(t, tl.Unlock(ctx, jobID, twoTargets))
}

func TestInMemoryLockUnlockDifferentJobID(t *testing.T) {
	tl := New()
	require.NoError(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, twoTargets))
	assert.Error(t, tl.Unlock(ctx, jobID+1, twoTargets))
}

func TestInMemoryRefreshLocks(t *testing.T) {
	tl := New()
	require.NoError(t, tl.RefreshLocks(ctx, jobID, twoTargets))
}

func TestInMemoryRefreshLocksTwice(t *testing.T) {
	tl := New()
	require.NoError(t, tl.RefreshLocks(ctx, jobID, twoTargets))
	assert.NoError(t, tl.RefreshLocks(ctx, jobID, twoTargets))
}

func TestInMemoryRefreshLocksOneThenTwo(t *testing.T) {
	tl := New()
	require.NoError(t, tl.RefreshLocks(ctx, jobID, oneTarget))
	assert.NoError(t, tl.RefreshLocks(ctx, jobID, twoTargets))
}

func TestInMemoryRefreshLocksTwoThenOne(t *testing.T) {
	tl := New()
	require.NoError(t, tl.RefreshLocks(ctx, jobID, twoTargets))
	assert.NoError(t, tl.RefreshLocks(ctx, jobID, oneTarget))
}

func TestRefreshMultiple(t *testing.T) {
	tl := New()
	require.NoError(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, twoTargets))
	time.Sleep(100 * time.Millisecond)
	// they are not expired yet, extend both
	require.NoError(t, tl.RefreshLocks(ctx, jobID, twoTargets))
	time.Sleep(150 * time.Millisecond)
	// if they were refreshed properly, they are still valid and attempts to get them must fail
	require.Error(t, tl.Lock(ctx, otherJobID, defaultJobTargetManagerAcquireTimeout, []*target.Target{&targetOne}))
	require.Error(t, tl.Lock(ctx, otherJobID, defaultJobTargetManagerAcquireTimeout, []*target.Target{&targetTwo}))
}

func TestLockingTransactional(t *testing.T) {
	tl := New()
	// lock the second target
	require.NoError(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, []*target.Target{&targetTwo}))
	// try to lock both with another owner (this fails as expected)
	require.Error(t, tl.Lock(ctx, jobID+1, defaultJobTargetManagerAcquireTimeout, twoTargets))
	// API says target one should remain unlocked because Lock() is transactional
	// this means it can be locked by the first owner
	require.NoError(t, tl.Lock(ctx, jobID, defaultJobTargetManagerAcquireTimeout, []*target.Target{&targetOne}))
}
