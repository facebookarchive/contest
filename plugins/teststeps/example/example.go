// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package example

import (
	"fmt"
	"math/rand"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "Example"

// Params this step accepts.
const (
	// Fail this percentage of targets at random.
	FailPctParam = "FailPct"
)

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
// consumes Targets in input and pipes them to the output or error channel
// with intermediate buffering.
type Step struct {
	failPct int64
}

// Name returns the name of the Step
func (ts Step) Name() string {
	return Name
}

func (ts *Step) shouldFail(t *target.Target) bool {
	if ts.failPct > 0 {
		roll := rand.Int63n(101)
		return (roll <= ts.failPct)
	}
	return false
}

// Run executes the example step.
func (ts *Step) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	f := func(ctx xcontext.Context, target *target.Target) error {
		ctx.Logger().Infof("Executing on target %s", target)
		// NOTE: you may want more robust error handling here, possibly just
		//       logging the error, or a retry mechanism. Returning an error
		//       here means failing the entire job.
		if err := ev.Emit(ctx, testevent.Data{EventName: StartedEvent, Target: target, Payload: nil}); err != nil {
			return fmt.Errorf("failed to emit start event: %v", err)
		}
		if ts.shouldFail(target) {
			if err := ev.Emit(ctx, testevent.Data{EventName: FailedEvent, Target: target, Payload: nil}); err != nil {
				return fmt.Errorf("failed to emit finished event: %v", err)
			}
			return fmt.Errorf("target failed")
		} else {
			if err := ev.Emit(ctx, testevent.Data{EventName: FinishedEvent, Target: target, Payload: nil}); err != nil {
				return fmt.Errorf("failed to emit failed event: %v", err)
			}
		}
		return nil
	}
	return teststeps.ForEachTarget(Name, ctx, ch, f)
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *Step) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	if params.GetOne(FailPctParam).String() != "" {
		if pct, err := params.GetInt(FailPctParam); err == nil {
			ts.failPct = pct
		} else {
			return fmt.Errorf("invalid FailPct: %w", err)
		}
	}
	return nil
}

// Resume tries to resume a previously interrupted test step. ExampleTestStep
// cannot resume.
func (ts *Step) Resume(ctx xcontext.Context, ch test.TestStepChannels, _ test.TestStepParameters, ev testevent.EmitterFetcher) error {
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
