// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package echo

import (
	"errors"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
var Name = "Echo"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

// Step implements an echo-style printing plugin.
type Step struct{}

// New initializes and returns a new EchoStep. It implements the TestStepFactory
// interface.
func New() test.TestStep {
	return &Step{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}

// ValidateParameters validates the parameters that will be passed to the Run
// and Resume methods of the test step.
func (e Step) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	if t := params.GetOne("text"); t.IsEmpty() {
		return errors.New("Missing 'text' field in echo parameters")
	}
	return nil
}

// Name returns the name of the Step
func (e Step) Name() string {
	return Name
}

// Run executes the step
func (e Step) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	for {
		select {
		case target, ok := <-ch.In:
			if !ok {
				return nil
			}
			ctx.Infof("Running on target %s with text '%s'", target, params.GetOne("text"))
			ch.Out <- test.TestStepResult{Target: target}
		case <-ctx.Done():
			return nil
		}
	}
}

// CanResume tells whether this step is able to resume.
func (e Step) CanResume() bool {
	return false
}

// Resume tries to resume a previously interrupted test step. EchoStep cannot
// resume.
func (e Step) Resume(ctx xcontext.Context, _ test.TestStepChannels, _ test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}
