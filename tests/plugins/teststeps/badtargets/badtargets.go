// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package badtargets

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
const Name = "BadTargets"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

type badTargets struct {
}

// Name returns the name of the Step
func (ts *badTargets) Name() string {
	return Name
}

// Run executes a step that messes up the flow of targets.
func (ts *badTargets) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	for {
		select {
		case tgt, ok := <-ch.In:
			if !ok {
				return nil
			}
			switch tgt.ID {
			case "TDrop":
				// ... crickets ...
			case "TGood":
				// We should not depend on pointer matching, so emit a copy.
				tgt2 := *tgt
				select {
				case ch.Out <- test.TestStepResult{Target: &tgt2}:
				case <-ctx.Done():
					return xcontext.ErrCanceled
				}
			case "TDup":
				select {
				case ch.Out <- test.TestStepResult{Target: tgt}:
				case <-ctx.Done():
					return xcontext.ErrCanceled
				}
				select {
				case ch.Out <- test.TestStepResult{Target: tgt}:
				case <-ctx.Done():
					return xcontext.ErrCanceled
				}
			case "TExtra":
				tgt2 := &target.Target{ID: "TExtra2"}
				select {
				case ch.Out <- test.TestStepResult{Target: tgt}:
				case <-ctx.Done():
					return xcontext.ErrCanceled
				}
				select {
				case ch.Out <- test.TestStepResult{Target: tgt2}:
				case <-ctx.Done():
					return xcontext.ErrCanceled
				}
			case "T1":
				// Mangle the returned target name.
				tgt2 := &target.Target{ID: tgt.ID + "XXX"}
				select {
				case ch.Out <- test.TestStepResult{Target: tgt2}:
				case <-ctx.Done():
					return xcontext.ErrCanceled
				}
			default:
				return fmt.Errorf("Unexpected target name: %q", tgt.ID)
			}
		case <-ctx.Done():
			return xcontext.ErrCanceled
		}
	}
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *badTargets) ValidateParameters(ctx xcontext.Context, params test.TestStepParameters) error {
	return nil
}

// New creates a new badTargets step
func New() test.TestStep {
	return &badTargets{}
}
