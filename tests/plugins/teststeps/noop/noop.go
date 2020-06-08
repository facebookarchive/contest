// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package noop

import (
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "Noop"

// Events defines the events that a Step is allow to emit
var Events = []event.Name{}

type noop struct {
}

// Name returns the name of the Step
func (ts *noop) Name() string {
	return Name
}

// Run executes a step which does never return.
func (ts *noop) Run(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.Emitter) error {
	for {
		select {
		case t := <-ch.In:
			if t == nil {
				return nil
			}
			ch.Out <- &target.Result{Target: t}
		case <-cancel:
			return nil
		case <-pause:
			return nil
		}
	}
}

// ValidateParameters validates the parameters associated to the Step
func (ts *noop) ValidateParameters(params test.StepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleStep
// cannot resume.
func (ts *noop) Resume(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *noop) CanResume() bool {
	return false
}

// New creates a new noop step
func New() test.Step {
	return &noop{}
}
