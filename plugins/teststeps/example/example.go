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
	"github.com/facebookincubator/contest/pkg/target"
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

// Events defines the events that a Step is allow to emit. Emitting an event
// that is not registered here will cause the plugin to terminate with an error.
var Events = []event.Name{StartedEvent, FinishedEvent, FailedEvent}

// Step is an example implementation of a Step which simply
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
func (ts *Step) Run(cancel, pause <-chan struct{}, ch test.StepChannels, _ test.StepParameters, ev testevent.Emitter) error {
	for {

		r := rand.Intn(3)
		select {
		case t := <-ch.In:
			if t == nil {
				return nil
			}
			log.Infof("Executing on target %s", t)
			// NOTE: you may want more robust error handling here, possibly just
			//       logging the error, or a retry mechanism. Returning an error
			//       here means failing the entire job.
			if err := ev.Emit(testevent.Data{EventName: StartedEvent, Target: t, Payload: nil}); err != nil {
				return fmt.Errorf("failed to emit start event: %v", err)
			}
			result := &target.Result{Target: t}
			if r == 1 {
				result.Err = fmt.Errorf("target failed")
			}

			select {
			case <-cancel:
			case <-pause:
				return nil
			case ch.Out <- result:
				if result.Err != nil {
					log.Infof("target %v failed", result.Target)
					if err := ev.Emit(testevent.Data{EventName: FailedEvent, Target: t, Payload: nil}); err != nil {
						log.Warningf("failed to emit failed event: %v", err)
					}
				} else {
					log.Infof("target %v succeded", result.Target)
					if err := ev.Emit(testevent.Data{EventName: FinishedEvent, Target: t, Payload: nil}); err != nil {
						log.Warningf("failed to emit finished event: %v", err)
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

// ValidateParameters validates the parameters associated to the Step
func (ts *Step) ValidateParameters(_ test.StepParameters) error {
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleStep
// cannot resume.
func (ts *Step) Resume(cancel, pause <-chan struct{}, ch test.StepChannels, _ test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *Step) CanResume() bool {
	return false
}

// New initializes and returns a new ExampleStep.
func New() test.Step {
	return &Step{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.StepFactory, []event.Name) {
	return Name, New, Events
}
