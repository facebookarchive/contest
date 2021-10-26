// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package readmeta

import (
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name defined the name of the plugin
var (
	Name = "readmeta"
)

// Readmeta implements the contest.TargetManager interface.
type Readmeta struct {
}

// ValidateAcquireParameters valides parameters that will be passed to Acquire.
func (r Readmeta) ValidateAcquireParameters(params []byte) (interface{}, error) {
	return nil, nil
}

// ValidateReleaseParameters valides parameters that will be passed to Release.
func (r Readmeta) ValidateReleaseParameters(params []byte) (interface{}, error) {
	return nil, nil
}

// Acquire implements contest.TargetManager.Acquire
func (t *Readmeta) Acquire(ctx xcontext.Context, jobID types.JobID, jobTargetManagerAcquireTimeout time.Duration, parameters interface{}, tl target.Locker) ([]*target.Target, error) {
	// test metadata exists
	jobID2, ok1 := types.JobIDFromContext(ctx)
	// note this must use panic to abort the test run, as this is a test for the job runner which ignore the actual outcome
	if jobID2 == 0 || !ok1 {
		panic("Unable to extract jobID from context")
	}
	runID, ok2 := types.RunIDFromContext(ctx)
	if runID == 0 || !ok2 {
		panic("Unable to extract runID from context")
	}
	// return one target so the test continues normally
	return []*target.Target{{ID: "testtarget123"}}, nil
}

// Release releases the acquired resources.
func (t *Readmeta) Release(ctx xcontext.Context, jobID types.JobID, targets []*target.Target, params interface{}) error {
	// test metadata exists
	jobID2, ok1 := types.JobIDFromContext(ctx)
	// note this must use panic to abort the test run, as this is a test for the job runner which ignore the actual outcome
	if jobID2 == 0 || !ok1 {
		panic("Unable to extract jobID from context")
	}
	runID, ok2 := types.RunIDFromContext(ctx)
	if runID == 0 || !ok2 {
		panic("Unable to extract runID from context")
	}
	return nil
}

// New builds a new Readmeta object.
func New() target.TargetManager {
	return &Readmeta{}
}

// Load returns the name and factory which are needed to register the
// TargetManager.
func Load() (string, target.TargetManagerFactory) {
	return Name, New
}
