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
var Name = "Noop"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

// Noop is the no-op target locker. It does nothing.
type Noop struct {
}

// Lock locks the specified targets by doing nothing.
func (tl Noop) Lock(_ types.JobID, targets []*target.Target) error {
	log.Infof("Locked %d targets by doing nothing", len(targets))
	return nil
}

// TryLock locks the specified targets by doing nothing.
func (tl Noop) TryLock(_ types.JobID, targets []*target.Target, limit uint) ([]string, error) {
	log.Infof("Trylocked %d targets by doing nothing", len(targets))
	res := make([]string, 0, len(targets))
	for _, t := range targets {
		res = append(res, t.ID)
	}
	return res, nil
}

// Unlock unlocks the specified targets by doing nothing.
func (tl Noop) Unlock(_ types.JobID, targets []*target.Target) error {
	log.Infof("Unlocked %d targets by doing nothing", len(targets))
	return nil
}

// RefreshLocks refreshes all the locks by the internal (non-existing) timeout,
// by flawlessly doing nothing.
func (tl Noop) RefreshLocks(jobID types.JobID, targets []*target.Target) error {
	log.Infof("All %d target locks are refreshed, since I had to do nothing", len(targets))
	return nil
}

// New initializes and returns a new ExampleTestStep.
func New(_ time.Duration) target.Locker {
	return &Noop{}
}
