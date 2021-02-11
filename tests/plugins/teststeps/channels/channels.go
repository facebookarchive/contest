// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package channels

import (
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "Channels"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

type channels struct {
}

// Name returns the name of the Step
func (ts *channels) Name() string {
	return Name
}

// Run executes a step which does never return.s
func (ts *channels) Run(ctx statectx.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	for target := range ch.In {
		ch.Out <- target
	}
	close(ch.Out)
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *channels) ValidateParameters(params test.TestStepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. Channels test step
// cannot resume.
func (ts *channels) Resume(ctx statectx.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *channels) CanResume() bool {
	return false
}

// New creates a new Channels step
func New() test.TestStep {
	return &channels{}
}
