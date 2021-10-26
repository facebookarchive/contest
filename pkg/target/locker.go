// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"time"

	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// locker defines the locking engine used by ConTest.
var locker Locker

// LockerFactory is a type representing a function which builds
// a Locker.
type LockerFactory func(time.Duration, time.Duration) Locker

// Locker defines an interface to lock and unlock targets. It is passed
// to TargetManager's Acquire and Release methods.
// TargetManagers are not required to lock targets, but they are allowed to.
// The framework will lock targets after Acquire returns, but if this fails
// due to a race, the job fails.
// Calling any of the functions with an empty list of targets is allowed and
// will return without error.
type Locker interface {
	// Lock locks the specified targets.
	// The job ID is the owner of the lock.
	// This function either succeeds and locks all the requested targets, or
	// leaves the existing locks untouched in case of conflicts.
	// Locks are reentrant, locking existing locks (with the same owner)
	// extends the deadline.
	// Passing empty list of targets is allowed and is a no-op.
	Lock(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*Target) error

	// TryLock attempts to lock up to limit of the given targets.
	// The job ID is the owner of the lock.
	// This function attempts to lock up to limit of the given targets,
	// and returns a list of target IDs that it was able to lock.
	// This function does not return an error if it was not able to lock any targets.
	// Locks are reentrant, locking existing locks (with the same owner)
	// extends the deadline.
	// Passing empty list of targets is allowed and is a no-op.
	TryLock(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*Target, limit uint) ([]string, error)

	// Unlock unlocks the specificied targets if they are held by the given owner.
	// Unlock allows expired locks by the same owner but unlocking a target that wasn't locked
	// or that is now or has since been locked by a different owner is a failure.
	// Passing empty list of targets is allowed and is a no-op.
	Unlock(ctx xcontext.Context, jobID types.JobID, targets []*Target) error

	// RefreshLocks extends existing locks on the given targets for the specified duration.
	// This call will fail if even a single target is not currently locked by the specified job.
	// Passing empty list of targets is allowed and is a no-op.
	RefreshLocks(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*Target) error

	// Close finalizes the locker and releases resources.
	// No API calls must be in flight when this is invoked or afterwards.
	Close() error
}

// SetLocker sets the desired lock engine for targets.
func SetLocker(newLocker Locker) {
	if locker != nil {
		locker.Close()
	}
	locker = newLocker
}

// GetLocker gets the desired lock engine for targets.
func GetLocker() Locker {
	return locker
}
