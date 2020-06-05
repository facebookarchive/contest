// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package panicstep

import (
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "Panic"

// Events defines the events that a Step is allow to emit
var Events = []event.Name{}

type panicStep struct {
}

// Name returns the name of the Step
func (ts *panicStep) Name() string {
	return Name
}

// Run executes the example step.
func (ts *panicStep) Run(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.Emitter) error {
	panic("panic step")
}

// ValidateParameters validates the parameters associated to the Step
func (ts *panicStep) ValidateParameters(params test.StepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleStep
// cannot resume.
func (ts *panicStep) Resume(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *panicStep) CanResume() bool {
	return false
}

// New creates a new panicStep
func New() test.Step {
	return &panicStep{}
}
