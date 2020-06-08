// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package randecho

import (
	"errors"
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
var Name = "RandEcho"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

// Events defines the events that a Step is allow to emit
var Events = []event.Name{}

// Step implements an echo-style printing plugin.
type Step struct{}

// New initializes and returns a new RandEcho. It implements the StepFactory
// interface.
func New() test.Step {
	return &Step{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.StepFactory, []event.Name) {
	return Name, New, Events
}

// ValidateParameters validates the parameters that will be passed to the Run
// and Resume methods of the test step.
func (e Step) ValidateParameters(params test.StepParameters) error {
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
func (e Step) Run(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.Emitter) error {
	for {
		select {
		case t := <-ch.In:
			if t == nil {
				// no more targets incoming
				return nil
			}
			r := rand.Intn(2)

			result := &target.Result{Target: t}
			var eventName event.Name
			if r == 0 {
				eventName = event.Name("TargetSucceeded")
				log.Infof("Run: target %s succeeded: %s", t, params.GetOne("text"))
			} else {
				eventName = event.Name("TargetFailed")
				log.Infof("Run: target %s failed: %s", t, params.GetOne("text"))
			}
			evData := testevent.Data{EventName: eventName, Target: t, Payload: nil}
			_ = ev.Emit(evData)

			ch.Out <- result
		case <-cancel:
			return nil
		case <-pause:
			return nil
		}
	}
}

// CanResume tells whether this step is able to resume.
func (e Step) CanResume() bool {
	return false
}

// Resume tries to resume a previously interrupted test step. RandEchoStep cannot
// resume.
func (e Step) Resume(cancel, pause <-chan struct{}, _ test.StepChannels, _ test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}
