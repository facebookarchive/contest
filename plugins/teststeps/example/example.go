// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package example

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name is the name used to look this plugin up.
var Name = "Example"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

// events that we may emit during the plugin's lifecycle. This is used in Events below.
// Note that you don't normally need to emit start/finish/cancellation events as
// these are emitted automatically by the framework.
const (
	StartedEvent  = event.Name("ExampleStartedEvent")
	FinishedEvent = event.Name("ExampleFinishedEvent")
	FailedEvent   = event.Name("ExampleFailedEvent")
)

// Events defines the events that a TestStep is allow to emit. Emitting an event
// that is not registered here will cause the plugin to terminate with an error.
var Events = []event.Name{StartedEvent, FinishedEvent, FailedEvent}

// Step is an example implementation of a TestStep which simply
// consumes Targets in input and pipes them to the output channel with intermediate
// buffering. It randomly decides if a Target has failed and forwards it on
// the err channel.
type Step struct {
}

// Name returns the name of the Step
func (ts Step) Name() string {
	return Name
}

// Run executes the example step.
func (ts *Step) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, _ test.TestStepParameters, ev testevent.Emitter) error {
	for {

		r := rand.Intn(3)
		select {
		case target := <-ch.In:
			if target == nil {
				return nil
			}
			log.Infof("Executing on target %s", target)
			// NOTE: you may want more robust error handling here, possibly just
			//       logging the error, or a retry mechanism. Returning an error
			//       here means failing the entire job.
			if err := ev.Emit(testevent.Data{EventName: StartedEvent, Target: target, Payload: nil}); err != nil {
				return fmt.Errorf("failed to emit start event: %v", err)
			}
			if r == 1 {
				select {
				case <-cancel:
				case <-pause:
					return nil
				case ch.Err <- cerrors.TargetError{Target: target, Err: fmt.Errorf("target failed")}:
					if err := ev.Emit(testevent.Data{EventName: FinishedEvent, Target: target, Payload: nil}); err != nil {
						return fmt.Errorf("failed to emit finished event: %v", err)
					}
				}
			} else {
				select {
				case <-cancel:
				case <-pause:
					return nil
				case ch.Out <- target:
					if err := ev.Emit(testevent.Data{EventName: FailedEvent, Target: target, Payload: nil}); err != nil {
						return fmt.Errorf("failed to emit failed event: %v", err)
					}
				}
			}
		case <-cancel:
			return nil
		case <-pause:
			return nil
		}
	}
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *Step) ValidateParameters(_ test.TestStepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleTestStep
// cannot resume.
func (ts *Step) Resume(cancel, pause <-chan struct{}, ch test.TestStepChannels, _ test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *Step) CanResume() bool {
	return false
}

// New initializes and returns a new ExampleTestStep.
func New() test.TestStep {
	return &Step{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}
