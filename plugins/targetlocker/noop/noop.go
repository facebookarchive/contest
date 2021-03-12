// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package noop implements a no-op target locker.
package noop

import (
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
var Name = "Noop"

// Noop is the no-op target locker. It does nothing.
type Noop struct {
}

// Lock locks the specified targets by doing nothing.
func (tl Noop) Lock(ctx xcontext.Context, _ types.JobID, _ time.Duration, targets []*target.Target) error {
	ctx.Logger().Infof("Locked %d targets by doing nothing", len(targets))
	return nil
}

// TryLock locks the specified targets by doing nothing.
func (tl Noop) TryLock(ctx xcontext.Context, _ types.JobID, _ time.Duration, targets []*target.Target, limit uint) ([]string, error) {
	ctx.Logger().Infof("Trylocked %d targets by doing nothing", len(targets))
	res := make([]string, 0, len(targets))
	for _, t := range targets {
		res = append(res, t.ID)
	}
	return res, nil
}

// Unlock unlocks the specified targets by doing nothing.
func (tl Noop) Unlock(ctx xcontext.Context, _ types.JobID, targets []*target.Target) error {
	ctx.Logger().Infof("Unlocked %d targets by doing nothing", len(targets))
	return nil
}

// RefreshLocks refreshes all the locks by the internal (non-existing) timeout,
// by flawlessly doing nothing.
func (tl Noop) RefreshLocks(ctx xcontext.Context, jobID types.JobID, targets []*target.Target) error {
	ctx.Logger().Infof("All %d target locks are refreshed, since I had to do nothing", len(targets))
	return nil
}

// New initializes and returns a new ExampleTestStep.
func New() target.Locker {
	return &Noop{}
}
