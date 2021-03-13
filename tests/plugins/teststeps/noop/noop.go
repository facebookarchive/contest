// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package noop

import (
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
var Name = "Noop"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

type noop struct {
}

// Name returns the name of the Step
func (ts *noop) Name() string {
	return Name
}

// Run executes a step which does never return.
func (ts *noop) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	for {
		select {
		case target := <-ch.In:
			if target == nil {
				return nil
			}
			ch.Out <- target
		case <-ctx.PausedOrDone():
			return nil
		}
	}
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *noop) ValidateParameters(params test.TestStepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleTestStep
// cannot resume.
func (ts *noop) Resume(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *noop) CanResume() bool {
	return false
}

// New creates a new noop step
func New() test.TestStep {
	return &noop{}
}
