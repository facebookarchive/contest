// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"time"

	"github.com/facebookincubator/contest/pkg/types"
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
	// The timeout is controlled by the locker plugin and set at construction time.
	// The job ID is the owner of the lock.
	// This function either succeeds and locks all the requested targets, or
	// leaves the existing locks untouched in case of conflicts.
	// Locks are reentrant, locking existing locks (with the same owner)
	// extends the deadline.
	Lock(types.JobID, []*Target) error
	// TryLock attempts to lock up to limit of the given targets.
	// The job ID is the owner of the lock.
	// This function attempts to lock up to limit of the given targets,
	// and returns a list of target IDs that it was able to lock.
	// This function does not return an error if it was not able to lock any targets.
	// Locks are reentrant, locking existing locks (with the same owner)
	// extends the deadline.
	TryLock(jobID types.JobID, targets []*Target, limit uint) ([]string, error)
	// Unlock unlocks the specificied targets if they are held by the given owner.
	// Unlock silently skips expired locks and targets that are not locked at all.
	// Unlock does not fail if a valid lock is held on one of the targets.
	// In these cases, a warning is printed, the foreign lock is left intact and
	// no error is returned.
	Unlock(types.JobID, []*Target) error
	// RefreshLocks locks or extends existing locks on the given targets.
	// This function offers the same behavior and guarantees as Lock,
	// except it uses a different timeout.
	// Note this means calling RefreshLocks on unlocked targets is allowed and
	// will (re-)acquire the lock.
	RefreshLocks(types.JobID, []*Target) error
}

// SetLocker sets the desired lock engine for targets.
func SetLocker(targetLocker Locker) {
	locker = targetLocker
}

// GetLocker gets the desired lock engine for targets.
func GetLocker() Locker {
	return locker
}
