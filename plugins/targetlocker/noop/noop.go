// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package noop implements a no-op target locker.
package noop

import (
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// Name is the name used to look this plugin up.
const Name = "Noop"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

// Noop is the no-op target locker. It does nothing.
type Noop struct {
}

// Lock locks the specified targets by doing nothing.
func (tl Noop) Lock(_ types.JobID, targets []*target.Target) error {
	log.Infof("Locked %d targets by doing nothing", len(targets))
	return nil
}

// Unlock unlocks the specified targets by doing nothing.
func (tl Noop) Unlock(_ types.JobID, targets []*target.Target) error {
	log.Infof("Unlocked %d targets by doing nothing", len(targets))
	return nil
}

// CheckLocks tells whether all the targets are locked. They all are, always. It
// also returns an array of the ones that are locked, and the ones that are not locked.
func (tl Noop) CheckLocks(jobID types.JobID, targets []*target.Target) ([]*target.Target, []*target.Target, error) {
	log.Infof("All %d targets are obviously locked, since I did nothing", len(targets))
	return targets, nil, nil
}

// RefreshLocks refreshes all the locks by the internal (non-existing) timeout,
// by flawlessly doing nothing.
func (tl Noop) RefreshLocks(jobID types.JobID, targets []*target.Target) error {
	log.Infof("All %d target locks are refreshed, since I had to do nothing", len(targets))
	return nil
}

// New initializes and returns a new noop target locker, which
// is just dummy implementation of the target.Locker interface,
// but does nothing in real.
func New(timeout time.Duration, _ string) (target.Locker, error) {
	return &Noop{}, nil
}

// Load returns the name and factory  which are needed to register the locker.
func Load() (string, target.LockerFactory) {
	return Name, New
}
