// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package badtargets

import (
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/test"
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
func (ts *badTargets) Run(ctx statectx.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	for {
		select {
		case target, ok := <-ch.In:
			if !ok {
				return nil
			}
			switch target.ID {
			case "TDrop":
				// ... crickets ...
			case "TGood":
				// We should not depend on pointer matching, so emit a copy.
				target2 := *target
				ch.Out <- &target2
			case "TDup":
				ch.Out <- target
				ch.Out <- target
			default:
				// Mangle the returned target name.
				target2 := *target
				target2.ID = target2.ID + "XXX"
				ch.Out <- &target2
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *badTargets) ValidateParameters(params test.TestStepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleTestStep
// cannot resume.
func (ts *badTargets) Resume(ctx statectx.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *badTargets) CanResume() bool {
	return false
}

// New creates a new badTargets step
func New() test.TestStep {
	return &badTargets{}
}
