// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package fail

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "Fail"

// Events defines the events that a Step is allow to emit
var Events = []event.Name{}

type fail struct {
}

// Name returns the name of the Step
func (ts *fail) Name() string {
	return Name
}

// Run executes a step which does never return.
func (ts *fail) Run(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.Emitter) error {
	for {
		select {
		case t := <-ch.In:
			if t == nil {
				return nil
			}
			ch.Out <- &target.Result{Target: t, Err: fmt.Errorf("Integration test failure for %v", t)}
		case <-cancel:
			return nil
		case <-pause:
			return nil
		}
	}
}

// ValidateParameters validates the parameters associated to the Step
func (ts *fail) ValidateParameters(params test.StepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleStep
// cannot resume.
func (ts *fail) Resume(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *fail) CanResume() bool {
	return false
}

// New creates a new noop step
func New() test.Step {
	return &fail{}
}
