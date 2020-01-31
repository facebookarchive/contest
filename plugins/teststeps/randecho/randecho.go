// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package randecho

import (
	"errors"
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
var Name = "RandEcho"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

// Step implements an echo-style printing plugin.
type Step struct{}

// New initializes and returns a new RandEcho. It implements the TestStepFactory
// interface.
func New() test.TestStep {
	return &Step{}
}

// ValidateParameters validates the parameters that will be passed to the Run
// and Resume methods of the test step.
func (e Step) ValidateParameters(params test.TestStepParameters) error {
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
func (e Step) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	for {
		select {
		case target := <-ch.In:
			if target == nil {
				// no more targets incoming
				return nil
			}
			r := rand.Intn(2)
			if r == 0 {
				evData := testevent.Data{
					EventName: event.Name("TargetSucceeded"),
					Target:    target,
					Payload:   nil,
				}
				_ = ev.Emit(evData)
				log.Infof("Run: target %s succeeded: %s", target, params.GetOne("text"))
				ch.Out <- target
			} else {
				evData := testevent.Data{
					EventName: event.Name("TargetFailed"),
					Target:    target,
					Payload:   nil,
				}
				_ = ev.Emit(evData)
				log.Infof("Run: target %s failed: %s", target, params.GetOne("text"))
				ch.Err <- cerrors.TargetError{Target: target, Err: fmt.Errorf("target randomly failed")}
			}
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
func (e Step) Resume(cancel, pause <-chan struct{}, _ test.TestStepChannels, _ test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}
