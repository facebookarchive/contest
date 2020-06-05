// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package noreturn

import (
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "NoReturn"

// Events defines the events that a Step is allow to emit
var Events = []event.Name{}

type noreturnStep struct {
}

// Name returns the name of the Step
func (ts *noreturnStep) Name() string {
	return Name
}

// Run executes a step which does never return.
func (ts *noreturnStep) Run(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.Emitter) error {
	for target := range ch.In {
		ch.Out <- target
	}
	channel := make(chan struct{})
	<-channel
	return nil
}

// ValidateParameters validates the parameters associated to the Step
func (ts *noreturnStep) ValidateParameters(params test.StepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleStep
// cannot resume.
func (ts *noreturnStep) Resume(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *noreturnStep) CanResume() bool {
	return false
}

// New creates a new noreturnStep which forwards targets before hanging
func New() test.Step {
	return &noreturnStep{}
}
