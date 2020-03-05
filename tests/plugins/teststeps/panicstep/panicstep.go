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

type panicStep struct {
}

// Name returns the name of the Step
func (ts *panicStep) Name() string {
	return Name
}

// Run executes the example step.
func (ts *panicStep) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	panic("panic step")
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *panicStep) ValidateParameters(params test.TestStepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleTestStep
// cannot resume.
func (ts *panicStep) Resume(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *panicStep) CanResume() bool {
	return false
}

// Factory implements test.TestStepFactory
type Factory struct{}

// New constructs and returns a "panic" implementation of test.TestStep
func (f *Factory) New() test.TestStep {
	return &panicStep{}
}

// Events defines the events that a TestStep is allow to emit
func (f *Factory) Events() []event.Name {
	return nil
}

// UniqueImplementationName returns the unique name of the implementation
func (f *Factory) UniqueImplementationName() string {
	return Name
}
