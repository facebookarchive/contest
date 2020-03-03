// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"time"

	"github.com/facebookincubator/contest/pkg/abstract"
	"github.com/facebookincubator/contest/pkg/types"
)

// locker defines the locking engine used by ConTest.
var locker Locker

// LockerFactory is a type representing a function which builds
// a Locker.
type LockerFactory interface {
	abstract.Factory

	// New returns an implementation of interface Locker.
	New(timeout time.Duration, implSpecificArgument string) (Locker, error)
}

// LockerFactories is a helper type to operate over multiple LockerFactory-es
type LockerFactories []LockerFactory

// ToAbstract returns the factories as abstract.Factories
//
// Go has no contracts (yet) / traits / whatever, and Go does not allow
// to convert slice of interfaces to slice of another interfaces
// without a loop, so we have to implement this method for each
// non-abstract-factories slice
//
// TODO: try remove it when this will be implemented:
//       https://github.com/golang/proposal/blob/master/design/go2draft-contracts.md
func (lockerFactories LockerFactories) ToAbstract() (result abstract.Factories) {
	for _, factory := range lockerFactories {
		result = append(result, factory)
	}
	return
}

// Locker defines an interface to lock and unlock targets. It is passed
// to TargetManager's Acquire and Release methods, and the target manager
// implementation is required to lock all the returned targets.
type Locker interface {
	// Lock locks the specified targets. The timeout must be handled by the
	// plugin, and configured at construction time by the plugin's
	// Factory.
	//
	// The job ID is the owner of the lock.
	// The underlying implementation is responsible for using the job ID as lock
	// owner, and for unlocking the already-locked ones in case of errors.
	Lock(types.JobID, []*Target) error
	// Unlock unlocks the specified targets, with the specified job ID as owner.
	// The underlying implementation is responsible of rejecting the operation if
	// the lock owner is not matching the job ID.
	Unlock(types.JobID, []*Target) error
	// RefreshLocks extends the lock. The amount of time the lock is extended
	// should be handled by the plugin, and configured at initialization time.
	// The request is rejected if the job ID does not match the one of the lock
	// owner.
	RefreshLocks(types.JobID, []*Target) error
	// CheckLocks returns whether all the targets are locked by the given job ID,
	// an array of locked targets, and an array of not-locked targets.
	CheckLocks(types.JobID, []*Target) ([]*Target, []*Target, error)
}

// SetLocker sets the desired lock engine for targets.
func SetLocker(targetLocker Locker) {
	locker = targetLocker
}

// GetLocker gets the desired lock engine for targets.
func GetLocker() Locker {
	return locker
}
