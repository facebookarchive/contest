// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package crash

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "Crash"

// Events defines the events that a Step is allow to emit
var Events = []event.Name{}

type crash struct {
}

// Name returns the name of the Step
func (ts *crash) Name() string {
	return Name
}

// Run executes a step which does never return.
func (ts *crash) Run(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.Emitter) error {
	return fmt.Errorf("Step crashed")
}

// ValidateParameters validates the parameters associated to the Step
func (ts *crash) ValidateParameters(params test.StepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleStep
// cannot resume.
func (ts *crash) Resume(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *crash) CanResume() bool {
	return false
}

// New creates a new noop step
func New() test.Step {
	return &crash{}
}
