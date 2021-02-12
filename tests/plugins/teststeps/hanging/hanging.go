// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package hanging

import (
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "Hanging"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

type hanging struct {
}

// Name returns the name of the Step
func (ts *hanging) Name() string {
	return Name
}

// Run executes a step which does never return.
func (ts *hanging) Run(ctx statectx.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	channel := make(chan struct{})
	<-channel
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *hanging) ValidateParameters(params test.TestStepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleTestStep
// cannot resume.
func (ts *hanging) Resume(ctx statectx.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *hanging) CanResume() bool {
	return false
}

// New creates a new hanging step
func New() test.TestStep {
	return &hanging{}
}
