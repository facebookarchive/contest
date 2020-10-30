// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration_storage

package dblocker

import (
	"os"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/plugins/targetlocker/dblocker"
	"github.com/stretchr/testify/assert"
)

var (
	jobID = types.JobID(123)

	targetOne  = target.Target{Name: "target001", ID: "001"}
	targetTwo  = target.Target{Name: "target002", ID: "002"}
	oneTarget  = []*target.Target{&targetOne}
	twoTargets = []*target.Target{&targetOne, &targetTwo}

	tl *dblocker.DBLocker
)

func TestMain(m *testing.M) {
	// tests reset the database, which makes the locker yell all the time,
	// disable for the integration tests
	logging.GetLogger("tests/integ")
	logging.Disable()

	dbURI := "contest:contest@tcp(mysql:3306)/contest_integ?parseTime=true"
	locker, err := dblocker.New(dbURI, 2*time.Second, 2*time.Second)
	if err != nil {
		panic(err)
	}
	tl = locker.(*dblocker.DBLocker)
	os.Exit(m.Run())
}

func TestNew(t *testing.T) {
	tl.ResetAllLocks()
	assert.NotNil(t, tl)
	assert.IsType(t, &dblocker.DBLocker{}, tl)
}

func TestLockInvalidJobIDAndNoTargets(t *testing.T) {
	tl.ResetAllLocks()
	assert.Error(t, tl.Lock(0, nil))
}

func TestLockValidJobIDAndNoTargets(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, nil))
}

func TestLockValidJobIDAndNoTargets2(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, []*target.Target{}))
}

func TestLockInvalidJobIDAndOneTarget(t *testing.T) {
	tl.ResetAllLocks()
	assert.Error(t, tl.Lock(0, oneTarget))
}

func TestLockValidJobIDAndEmptyIDTarget(t *testing.T) {
	tl.ResetAllLocks()
	assert.Error(t, tl.Lock(jobID, []*target.Target{&target.Target{Name: "test", ID: ""}}))
}

func TestLockValidJobIDAndOneTarget(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, oneTarget))
}

func TestLockValidJobIDAndTwoTargets(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, twoTargets))
}

func TestLockReentrantLock(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, twoTargets))
	assert.NoError(t, tl.Lock(jobID, oneTarget))
	assert.NoError(t, tl.Lock(jobID, twoTargets))
}

func TestLockReentrantLockDifferentJobID(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, twoTargets))
	assert.Error(t, tl.Lock(jobID+1, twoTargets))
	assert.Error(t, tl.Lock(jobID+1, oneTarget))
}

func TestUnlockInvalidJobIDAndNoTargets(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Unlock(jobID, nil))
}

func TestUnlockValidJobIDAndNoTargets(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Unlock(jobID, nil))
}

func TestUnlockInvalidJobIDAndOneTarget(t *testing.T) {
	tl.ResetAllLocks()
	assert.Error(t, tl.Unlock(0, oneTarget))
}

func TestUnlockValidJobIDAndOneTarget(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Unlock(jobID, oneTarget))
}

func TestUnlockValidJobIDAndTwoTargets(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Unlock(jobID, twoTargets))
}

func TestLockUnlockSameJobID(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, twoTargets))
	assert.NoError(t, tl.Unlock(jobID, twoTargets))
}

func TestLockUnlockDifferentJobID(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, twoTargets))
	// this does not error, but will also not release the lock...
	assert.NoError(t, tl.Unlock(jobID+1, twoTargets))
	// ... so it cannot be acquired by job+1
	assert.Error(t, tl.Lock(jobID+1, twoTargets))
}

func TestTryLockOne(t *testing.T) {
	tl.ResetAllLocks()
	res, err := tl.TryLock(jobID, oneTarget)
	assert.NoError(t, err)
	assert.Equal(t, oneTarget[0].ID, res[0])
}

func TestTryLockTwo(t *testing.T) {
	tl.ResetAllLocks()
	res, err := tl.TryLock(jobID, twoTargets)
	assert.NoError(t, err)
	// order is not guaranteed
	assert.Contains(t, res, twoTargets[0].ID)
	assert.Contains(t, res, twoTargets[1].ID)
}

func TestTryLockOneOfTwo(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, oneTarget))
	// now tryLock both with other ID
	res, err := tl.TryLock(jobID+1, twoTargets)
	assert.NoError(t, err)
	// should have locked 1 but not 0
	assert.NotContains(t, res, twoTargets[0].ID)
	assert.Contains(t, res, twoTargets[1].ID)
}

func TestTryLockNoneOfTwo(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, twoTargets))
	// now tryLock both with other ID
	res, err := tl.TryLock(jobID+1, twoTargets)
	// should have locked zero targets, but no error
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestRefreshLocks(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
}

func TestRefreshLocksTwice(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
}

func TestRefreshLocksOneThenTwo(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.RefreshLocks(jobID, oneTarget))
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
}

func TestRefreshLocksTwoThenOne(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
	assert.NoError(t, tl.RefreshLocks(jobID, oneTarget))
}

func TestLockExpiry(t *testing.T) {
	tl.ResetAllLocks()
	assert.NoError(t, tl.Lock(jobID, twoTargets))
	// getting them immediately fails for other owner
	assert.Error(t, tl.Lock(jobID+1, twoTargets))
	time.Sleep(3 * time.Second)
	// expired, now it should work
	assert.NoError(t, tl.Lock(jobID+1, twoTargets))
}

func TestRefreshMultiple(t *testing.T) {
	// not super happy with this test, it is timing sensitive
	tl.ResetAllLocks()
	// now for the actual test
	assert.NoError(t, tl.Lock(jobID, twoTargets))
	time.Sleep(1500 * time.Millisecond)
	// they are not expired yet, extend both
	assert.NoError(t, tl.RefreshLocks(jobID, twoTargets))
	time.Sleep(1 * time.Second)
	// if they were refreshed properly, they are still valid and attempts to get them must fail
	assert.Error(t, tl.Lock(jobID+1, []*target.Target{&targetOne}))
	assert.Error(t, tl.Lock(jobID+1, []*target.Target{&targetTwo}))
}

func TestLockingTransactional(t *testing.T) {
	tl.ResetAllLocks()
	// lock the second target
	assert.NoError(t, tl.Lock(jobID, []*target.Target{&targetTwo}))
	// try to lock both with another owner (this fails as expected)
	assert.Error(t, tl.Lock(jobID+1, twoTargets))
	// API says target one should remain unlocked because Lock() is transactional
	// this means it can be locked by the first owner
	assert.NoError(t, tl.Lock(jobID, []*target.Target{&targetOne}))
}
