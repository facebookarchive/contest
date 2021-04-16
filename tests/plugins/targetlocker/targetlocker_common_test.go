// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetlocker

import (
	"time"

	"github.com/benbjohnson/clock"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/tests/common/goroutine_leak_check"
)

var (
	job1 = types.JobID(1)
	job2 = types.JobID(2)

	shortTimeout   = 2 * time.Minute
	defaultTimeout = 5 * time.Minute

	target1    = []*target.Target{&target.Target{ID: "001"}}
	target2    = []*target.Target{&target.Target{ID: "002"}}
	twoTargets = []*target.Target{target1[0], target2[0]}
	allTargets = []*target.Target{target1[0], target2[0], &target.Target{ID: "003"}, &target.Target{ID: "004"}}

	ctx = logrusctx.NewContext(logger.LevelDebug, logging.DefaultOptions()...)
)

type TargetLockerTestSuite struct {
	suite.Suite

	tl    target.Locker
	clock *clock.Mock
}

func (ts *TargetLockerTestSuite) TearDownSuite() {
	time.Sleep(20 * time.Millisecond)
	if err := goroutine_leak_check.CheckLeakedGoRoutines(); err != nil {
		ts.T().Errorf("%s", err)
	}
}

func (ts *TargetLockerTestSuite) TestLockInvalidJobIDAndNoTargets() {
	require.Error(ts.T(), ts.tl.Lock(ctx, 0, defaultTimeout, nil))
}

func (ts *TargetLockerTestSuite) TestLockValidJobIDAndNoTargets() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, nil))
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, []*target.Target{}))
}

func (ts *TargetLockerTestSuite) TestLockInvalidJobIDAndOneTarget() {
	require.Error(ts.T(), ts.tl.Lock(ctx, 0, defaultTimeout, target1))
}

func (ts *TargetLockerTestSuite) TestLockValidJobIDAndInvalidTarget() {
	require.Error(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, []*target.Target{&target.Target{ID: ""}}))
}

func (ts *TargetLockerTestSuite) TestLockValidJobIDAndOneTarget() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, target1))
}

func (ts *TargetLockerTestSuite) TestLockValidJobIDAndTwoTargets() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
}

func (ts *TargetLockerTestSuite) TestLockReentrantLock() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, allTargets))
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, target1))
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, allTargets))
}

func (ts *TargetLockerTestSuite) TestLockReentrantLockDifferentJobID() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, allTargets))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, twoTargets))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, allTargets))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, []*target.Target{allTargets[3]}))
}

func (ts *TargetLockerTestSuite) TestTryLockOne() {
	res, err := ts.tl.TryLock(ctx, job1, defaultTimeout, target1, 1)
	require.NoError(ts.T(), err)
	require.Equal(ts.T(), target1[0].ID, res[0])
}

func (ts *TargetLockerTestSuite) TestTryLockTwo() {
	res, err := ts.tl.TryLock(ctx, job1, defaultTimeout, twoTargets, 2)
	require.NoError(ts.T(), err)
	// order is not guaranteed
	require.Contains(ts.T(), res, twoTargets[0].ID)
	require.Contains(ts.T(), res, twoTargets[1].ID)
}

func (ts *TargetLockerTestSuite) TestTryLockSome() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	res, err := ts.tl.TryLock(ctx, job2, defaultTimeout, allTargets, uint(len(allTargets)))
	require.NoError(ts.T(), err)
	// asked for all, got some
	require.Equal(ts.T(), 2, len(res))
	require.Contains(ts.T(), res, allTargets[2].ID)
	require.Contains(ts.T(), res, allTargets[3].ID)
}

func (ts *TargetLockerTestSuite) TestTryLockSameJob() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	// job is the same, so we get all 4
	res, err := ts.tl.TryLock(ctx, job1, defaultTimeout, allTargets, uint(len(allTargets)))
	require.NoError(ts.T(), err)
	require.Equal(ts.T(), 4, len(res))
	require.Contains(ts.T(), res, allTargets[0].ID)
	require.Contains(ts.T(), res, allTargets[1].ID)
	require.Contains(ts.T(), res, allTargets[2].ID)
	require.Contains(ts.T(), res, allTargets[3].ID)
}

func (ts *TargetLockerTestSuite) TestTryLockZeroLimited() {
	// only request one
	res, err := ts.tl.TryLock(ctx, job1, defaultTimeout, twoTargets, 0)
	require.NoError(ts.T(), err)
	// it is allowed to set the limit to zero
	require.Equal(ts.T(), len(res), 0)
}

func (ts *TargetLockerTestSuite) TestTryLockTwoHigherLimit() {
	// limit is just an upper bound, can be higher
	res, err := ts.tl.TryLock(ctx, job1, defaultTimeout, twoTargets, 100)
	require.NoError(ts.T(), err)
	// order is not guaranteed
	require.Contains(ts.T(), res, twoTargets[0].ID)
	require.Contains(ts.T(), res, twoTargets[1].ID)
}

func (ts *TargetLockerTestSuite) TestTryLockOneLimited() {
	// only request one
	res, err := ts.tl.TryLock(ctx, job1, defaultTimeout, twoTargets, 1)
	require.NoError(ts.T(), err)
	require.Equal(ts.T(), len(res), 1)
	// API doesn't require it, but locker guarantees order
	// so the first one should have been locked,
	// the second not because limit was 1
	require.Contains(ts.T(), res, twoTargets[0].ID)
	require.NotContains(ts.T(), res, twoTargets[1].ID)
}

func (ts *TargetLockerTestSuite) TestTryLockOneOfTwo() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, target1))
	// now tryLock both with other ID
	res, err := ts.tl.TryLock(ctx, job2, defaultTimeout, twoTargets, 2)
	require.NoError(ts.T(), err)
	// should have locked 1 but not 0
	require.NotContains(ts.T(), res, twoTargets[0].ID)
	require.Contains(ts.T(), res, twoTargets[1].ID)
}

func (ts *TargetLockerTestSuite) TestTryLockNoneOfTwo() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	// now tryLock both with other ID
	res, err := ts.tl.TryLock(ctx, job2, defaultTimeout, twoTargets, 2)
	// should have locked zero targets, but no error
	require.NoError(ts.T(), err)
	require.Empty(ts.T(), res)
}

func (ts *TargetLockerTestSuite) TestUnlockInvalidJobIDAndNoTargets() {
	require.Error(ts.T(), ts.tl.Unlock(ctx, 0, nil))
	require.Error(ts.T(), ts.tl.Unlock(ctx, 0, []*target.Target{}))
}

func (ts *TargetLockerTestSuite) TestUnlockValidJobIDAndNoTargets() {
	require.NoError(ts.T(), ts.tl.Unlock(ctx, job1, nil))
	require.NoError(ts.T(), ts.tl.Unlock(ctx, job1, []*target.Target{}))
}

func (ts *TargetLockerTestSuite) TestUnlockInvalidJobIDAndOneTarget() {
	require.Error(ts.T(), ts.tl.Unlock(ctx, 0, target1))
}

func (ts *TargetLockerTestSuite) TestUnlockValidJobIDAndOneTarget() {
	require.Error(ts.T(), ts.tl.Unlock(ctx, job1, target1))
}

func (ts *TargetLockerTestSuite) TestUnlockValidJobIDAndTwoTargets() {
	require.Error(ts.T(), ts.tl.Unlock(ctx, job1, twoTargets))
}

func (ts *TargetLockerTestSuite) TestUnlockUnlockTwice() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, target1))
	require.NoError(ts.T(), ts.tl.Unlock(ctx, job1, target1))
	require.Error(ts.T(), ts.tl.Unlock(ctx, job1, target1))
}

func (ts *TargetLockerTestSuite) TestUnlockReentrantLockDifferentJobID() {
	require.Error(ts.T(), ts.tl.Unlock(ctx, job1, twoTargets))
	require.Error(ts.T(), ts.tl.Unlock(ctx, job2, twoTargets))
}

func (ts *TargetLockerTestSuite) TestLockUnlockSameJobID() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.Unlock(ctx, job1, twoTargets))
	require.NoError(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, twoTargets))
}

func (ts *TargetLockerTestSuite) TestUnlockTransactional() {
	// lock both
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	// unlock second
	require.NoError(ts.T(), ts.tl.Unlock(ctx, job1, target2))
	// first is still locked, second is not
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.NoError(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target2))
	// now try unlock both by job1, this fails because we don't own t2
	require.Error(ts.T(), ts.tl.Unlock(ctx, job1, twoTargets))
	// t1 remains locked.
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
}

func (ts *TargetLockerTestSuite) TestLockUnlockDifferentJobID() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	require.Error(ts.T(), ts.tl.Unlock(ctx, job2, twoTargets))
	// targets remain locked by job1
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
}

func (ts *TargetLockerTestSuite) TestRefreshValidJobIDAndNoTargets() {
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, nil))
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, []*target.Target{}))
}

func (ts *TargetLockerTestSuite) TestRefreshNonexistentLocks() {
	require.Error(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	// targets are not locked after this
	require.NoError(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, twoTargets))
}

func (ts *TargetLockerTestSuite) TestRefreshExpiredLocks() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, shortTimeout, twoTargets))
	ts.clock.Add(defaultTimeout)
	// oops, we're late, but that's ok since job id is the same.
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	// both targets are re-locked by job1
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target2))
}

func (ts *TargetLockerTestSuite) TestRefreshExpiredLocksDifferentJob() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, shortTimeout, twoTargets))
	ts.clock.Add(defaultTimeout)
	// these are expired locks from some other job, don't allow refreshing them.
	require.Error(ts.T(), ts.tl.RefreshLocks(ctx, job2, defaultTimeout, twoTargets))
	// but they are available to be locked.
	require.NoError(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, twoTargets))
}

func (ts *TargetLockerTestSuite) TestRefreshLocksTwice() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	// both targets are still locked by job1
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target2))
}

func (ts *TargetLockerTestSuite) TestRefreshLocksOneThenTwo() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, target1))
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	// both targets are still locked by job1
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target2))
}

func (ts *TargetLockerTestSuite) TestRefreshLocksTwoThenOne() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, target1))
	// both targets are still locked by job1
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target2))
}

func (ts *TargetLockerTestSuite) TestNoRefreshAfterUnlock() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	require.NoError(ts.T(), ts.tl.Unlock(ctx, job1, target1))
	require.Error(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	// refresh failed but second target is still locked.
	require.NoError(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target2))
}

func (ts *TargetLockerTestSuite) TestNoRefreshDifferentOwner() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, target1))
	require.NoError(ts.T(), ts.tl.Lock(ctx, job2, shortTimeout, target2))
	require.Error(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
}

func (ts *TargetLockerTestSuite) TestRefreshMultiple() {
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, twoTargets))
	ts.clock.Add(shortTimeout)
	// they are not expired yet, extend both
	require.NoError(ts.T(), ts.tl.RefreshLocks(ctx, job1, defaultTimeout, twoTargets))
	ts.clock.Add(defaultTimeout - 1)
	// if they were refreshed properly, they are still valid and attempts to get them must fail
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target1))
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, target2))
}

func (ts *TargetLockerTestSuite) TestLockingTransactional() {
	// lock the second target
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, target2))
	// try to lock both with another owner (this fails as expected)
	require.Error(ts.T(), ts.tl.Lock(ctx, job2, defaultTimeout, twoTargets))
	// API says target one should remain unlocked because Lock() is transactional
	// this means it can be locked by the first owner
	require.NoError(ts.T(), ts.tl.Lock(ctx, job1, defaultTimeout, target1))
}
