// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package randecho

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "RandEcho"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

// Step implements an echo-style printing plugin.
type Step struct{}

// New initializes and returns a new RandEcho. It implements the TestStepFactory
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
func (e Step) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	return teststeps.ForEachTarget(Name, ctx, ch,
		func(ctx xcontext.Context, target *target.Target) error {
			r := rand.Intn(2)
			if r == 0 {
				evData := testevent.Data{
					EventName: event.Name("TargetSucceeded"),
					Target:    target,
					Payload:   nil,
				}
				_ = ev.Emit(ctx, evData)
				ctx.Infof("Run: target %s succeeded: %s", target, params.GetOne("text"))
				return nil
			} else {
				evData := testevent.Data{
					EventName: event.Name("TargetFailed"),
					Target:    target,
					Payload:   nil,
				}
				_ = ev.Emit(ctx, evData)
				ctx.Infof("Run: target %s failed: %s", target, params.GetOne("text"))
				return fmt.Errorf("target randomly failed")
			}
		},
	)
}
