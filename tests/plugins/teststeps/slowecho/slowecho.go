// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package slowecho

import (
	"errors"
	"strconv"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "SlowEcho"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

// Step implements an echo-style printing plugin.
type Step struct {
}

// New initializes and returns a new EchoStep. It implements the TestStepFactory
// interface.
func New() test.TestStep {
	return &Step{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}

// Name returns the name of the Step
func (e *Step) Name() string {
	return Name
}

func sleepTime(secStr string) (time.Duration, error) {
	seconds, err := strconv.ParseFloat(secStr, 64)
	if err != nil {
		return 0, err
	}
	if seconds < 0 {
		return 0, errors.New("seconds cannot be negative in slowecho parameters")
	}
	return time.Duration(seconds*1000) * time.Millisecond, nil

}

// ValidateParameters validates the parameters that will be passed to the Run
// and Resume methods of the test step.
func (e *Step) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	if t := params.GetOne("text"); t.IsEmpty() {
		return errors.New("missing 'text' field in slowecho parameters")
	}
	secStr := params.GetOne("sleep")
	if secStr.IsEmpty() {
		return errors.New("missing 'sleep' field in slowecho parameters")
	}

	_, err := sleepTime(secStr.String())
	if err != nil {
		return err
	}
	return nil
}

// Run executes the step
func (e *Step) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	sleep, err := sleepTime(params.GetOne("sleep").String())
	if err != nil {
		return err
	}
	f := func(ctx xcontext.Context, t *target.Target) error {
		ctx.Infof("Waiting %v for target %s", sleep, t.ID)
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			ctx.Infof("Returning because cancellation is requested")
			return xcontext.ErrCanceled
		}
		ctx.Infof("target %s: %s", t, params.GetOne("text"))
		return nil
	}
	return teststeps.ForEachTarget(Name, ctx, ch, f)
}
