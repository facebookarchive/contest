// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Same as targetlist but verifies that state survives from Acquire to Resume.

package targetlist_with_state

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name defined the name of the plugin
var Name = "TargetListWithState"

// AcquireParameters contains the parameters necessary to acquire targets.
type AcquireParameters struct {
	Targets []*target.Target
}

// ReleaseParameters contains the parameters necessary to release targets.
type ReleaseParameters struct {
}

// TargetListWithState implements the contest.TargetManager interface.
type TargetListWithState struct {
}

// ValidateAcquireParameters valides parameters that will be passed to Acquire.
func (tlws TargetListWithState) ValidateAcquireParameters(params []byte) (interface{}, error) {
	var ap AcquireParameters
	if err := json.Unmarshal(params, &ap); err != nil {
		return nil, err
	}
	return ap, nil
}

// ValidateReleaseParameters valides parameters that will be passed to Release.
func (tlws TargetListWithState) ValidateReleaseParameters(params []byte) (interface{}, error) {
	return nil, nil
}

// Acquire implements contest.TargetManager.Acquire
func (tlws *TargetListWithState) Acquire(ctx xcontext.Context, jobID types.JobID, jobTargetManagerAcquireTimeout time.Duration, parameters interface{}, tl target.Locker) ([]*target.Target, error) {
	acquireParameters, ok := parameters.(AcquireParameters)
	if !ok {
		return nil, fmt.Errorf("Acquire expects %T object, got %T", acquireParameters, parameters)
	}

	if err := tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, acquireParameters.Targets); err != nil {
		ctx.Warnf("Failed to lock %d targets: %v", len(acquireParameters.Targets), err)
		return nil, err
	}
	var tt []*target.Target
	for _, tgt := range acquireParameters.Targets {
		tc := *tgt
		tc.TargetManagerState = json.RawMessage([]byte(fmt.Sprintf(`{"token":"%d-%s"}`, jobID, tc.ID)))
		tt = append(tt, &tc)
	}
	ctx.Infof("Acquired %d targets", tt)
	return tt, nil
}

// Release releases the acquired resources.
func (tlws *TargetListWithState) Release(ctx xcontext.Context, jobID types.JobID, targets []*target.Target, params interface{}) error {
	// Validate tokens to make sure state was passed correctly.
	for _, t := range targets {
		actualState := string(t.TargetManagerState)
		expectedState := fmt.Sprintf(`{"token":"%d-%s"}`, jobID, t.ID)
		if actualState != expectedState {
			panic(fmt.Sprintf("state mismatch: expected %q, got %q", expectedState, actualState))
		}
	}
	ctx.Infof("Released %d targets", len(targets))
	return nil
}

// New builds a new TargetListWithState object.
func New() target.TargetManager {
	return &TargetListWithState{}
}

// Load returns the name and factory which are needed to register the
// TargetManager.
func Load() (string, target.TargetManagerFactory) {
	return Name, New
}
