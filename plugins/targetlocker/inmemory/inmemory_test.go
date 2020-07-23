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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	jobID = types.JobID(123)

	noTarget   = []*target.Target{}
	targetOne  = target.Target{Name: "target001", ID: "001"}
	targetTwo  = target.Target{Name: "target002", ID: "002"}
	oneTarget  = []*target.Target{&targetOne}
	twoTargets = []*target.Target{&targetOne, &targetTwo}
)

func TestInMemoryNew(t *testing.T) {
	tl := New(time.Second)
	require.NotNil(t, tl)
	require.IsType(t, &InMemory{}, tl)
}

func TestInMemoryLockInvalidJobIDAndNoTargets(t *testing.T) {
	tl := New(time.Second)
	assert.Error(t, tl.Lock(0, nil))
}

func TestInMemoryLockValidJobIDAndNoTargets(t *testing.T) {
	tl := New(time.Second)
	assert.Error(t, tl.Lock(jobID, nil))
}

func TestInMemoryLockInvalidJobIDAndOneTarget(t *testing.T) {
	tl := New(time.Second)
	assert.Error(t, tl.Lock(0, oneTarget))
}

func TestInMemoryLockValidJobIDAndOneTarget(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.Lock(jobID, oneTarget))
}

func TestInMemoryLockValidJobIDAndTwoTargets(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.Lock(jobID, twoTargets))
}

func TestInMemoryLockReentrantLock(t *testing.T) {
	tl := New(10 * time.Second)
	require.NoError(t, tl.Lock(jobID, twoTargets))
	require.NoError(t, tl.Lock(jobID, twoTargets))
}

func TestInMemoryLockReentrantLockDifferentJobID(t *testing.T) {
	tl := New(10 * time.Second)
	require.NoError(t, tl.Lock(jobID, twoTargets))
	require.Error(t, tl.Lock(jobID+1, twoTargets))
}

func TestInMemoryUnlockInvalidJobIDAndNoTargets(t *testing.T) {
	tl := New(time.Second)
	assert.Error(t, tl.Unlock(jobID, nil))
}

func TestInMemoryUnlockValidJobIDAndNoTargets(t *testing.T) {
	tl := New(time.Second)
	assert.Error(t, tl.Unlock(jobID, nil))
}

func TestInMemoryUnlockInvalidJobIDAndOneTarget(t *testing.T) {
	tl := New(time.Second)
	assert.Error(t, tl.Unlock(0, oneTarget))
}

func TestInMemoryUnlockValidJobIDAndOneTarget(t *testing.T) {
	tl := New(time.Second)
	require.Error(t, tl.Unlock(jobID, oneTarget))
}

func TestInMemoryUnlockValidJobIDAndTwoTargets(t *testing.T) {
	tl := New(time.Second)
	require.Error(t, tl.Unlock(jobID, twoTargets))
}

func TestInMemoryUnlockUnlockTwice(t *testing.T) {
	tl := New(time.Second)
	err := tl.Unlock(jobID, oneTarget)
	log.Print(err)
	assert.Error(t, err)
	assert.Error(t, tl.Unlock(jobID, oneTarget))
}

func TestInMemoryUnlockReentrantLockDifferentJobID(t *testing.T) {
	tl := New(time.Second)
	require.Error(t, tl.Unlock(jobID, twoTargets))
	assert.Error(t, tl.Unlock(jobID+1, twoTargets))
}

func TestInMemoryLockUnlockSameJobID(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.Lock(jobID, twoTargets))
	assert.NoError(t, tl.Unlock(jobID, twoTargets))
}

func TestInMemoryLockUnlockDifferentJobID(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.Lock(jobID, twoTargets))
	assert.Error(t, tl.Unlock(jobID+1, twoTargets))
}

func TestInMemoryCheckLocksNoneLocked(t *testing.T) {
	tl := New(time.Second)
	allLocked, locked, notLocked := tl.CheckLocks(jobID, twoTargets)
	assert.False(t, allLocked)
	assert.Equal(t, noTarget, locked)
	assert.Equal(t, twoTargets, notLocked)
}

func TestInMemoryCheckLocksAllLocked(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.Lock(jobID, twoTargets))
	allLocked, locked, notLocked := tl.CheckLocks(jobID, twoTargets)
	assert.True(t, allLocked)
	assert.Equal(t, twoTargets, locked)
	assert.Equal(t, noTarget, notLocked)
}

func TestInMemoryCheckLocksSomeLocked(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.Lock(jobID, []*target.Target{&targetOne}))
	allLocked, locked, notLocked := tl.CheckLocks(jobID, []*target.Target{&targetOne, &targetTwo})
	assert.False(t, allLocked)
	assert.Equal(t, []*target.Target{&targetOne}, locked)
	assert.Equal(t, []*target.Target{&targetTwo}, notLocked)
}

func TestInMemoryCheckLocksMultipleOwners(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.Lock(jobID, []*target.Target{&targetOne}))
	require.NoError(t, tl.Lock(jobID+1, []*target.Target{&targetTwo}))
	// owner 1
	allLocked, locked, notLocked := tl.CheckLocks(jobID, []*target.Target{&targetOne, &targetTwo})
	assert.False(t, allLocked)
	assert.Equal(t, []*target.Target{&targetOne}, locked)
	assert.Equal(t, []*target.Target{&targetTwo}, notLocked)
	// owner 2
	allLocked, locked, notLocked = tl.CheckLocks(jobID+1, []*target.Target{&targetOne, &targetTwo})
	assert.False(t, allLocked)
	assert.Equal(t, []*target.Target{&targetTwo}, locked)
	assert.Equal(t, []*target.Target{&targetOne}, notLocked)
}

func TestInMemoryRefreshLocks(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.RefreshLocks(jobID, twoTargets))
}

func TestInMemoryRefreshLocksTwice(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.RefreshLocks(jobID, twoTargets))
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
}

func TestInMemoryRefreshLocksOneThenTwo(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.RefreshLocks(jobID, oneTarget))
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
}

func TestInMemoryRefreshLocksTwoThenOne(t *testing.T) {
	tl := New(time.Second)
	require.NoError(t, tl.RefreshLocks(jobID, twoTargets))
	assert.NoError(t, tl.RefreshLocks(jobID, oneTarget))
}
