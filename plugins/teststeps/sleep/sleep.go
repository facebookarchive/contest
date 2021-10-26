// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package sleep

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "Sleep"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

// sleepStep implements an echo-style printing plugin.
type sleepStep struct {
}

// New initializes and returns a new EchoStep. It implements the TestStepFactory
// interface.
func New() test.TestStep {
	return &sleepStep{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}

func getDuration(params test.TestStepParameters) (time.Duration, error) {
	durP := params.GetOne("duration")
	if durP.IsEmpty() {
		return 0, errors.New("Missing 'duration' field in sleep parameters")
	}
	dur, err := time.ParseDuration(durP.String())
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", durP.String(), err)
	}
	return dur, nil
}

// ValidateParameters validates the parameters that will be passed to the Run
// and Resume methods of the test step.
func (ss *sleepStep) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	_, err := getDuration(params)
	return err
}

// Name returns the name of the Step
func (ss *sleepStep) Name() string {
	return Name
}

type sleepStepData struct {
	DeadlineMS int64 `json:"D"`
}

// Run executes the step
func (ss *sleepStep) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	dur, err := getDuration(params)
	if err != nil {
		return nil, err
	}
	fn := func(ctx xcontext.Context, target *teststeps.TargetWithData) error {
		var deadline time.Time
		// copy, can be different per target
		var sleepTime = dur

		// handle resume
		if target.Data != nil {
			ssd := sleepStepData{}
			if err := json.Unmarshal(target.Data, &ssd); err != nil {
				return fmt.Errorf("invalid resume state: %w", err)
			}
			deadline = time.Unix(ssd.DeadlineMS/1000, (ssd.DeadlineMS%1000)*1000000)
			sleepTime = time.Until(deadline)
			ctx.Debugf("restored with %v unix, in %s", ssd.DeadlineMS, time.Until(deadline))
		} else {
			deadline = time.Now().Add(dur)
		}

		// now sleep
		select {
		case <-time.After(sleepTime):
			return nil
		case <-ctx.Until(xcontext.ErrPaused):
			ctx.Debugf("%s: Paused with %s left", target.Target, time.Until(deadline))
			ssd := &sleepStepData{
				DeadlineMS: deadline.UnixNano() / 1000000,
			}
			var err error
			target.Data, err = json.Marshal(ssd)
			if err != nil {
				return err
			}
			return xcontext.ErrPaused
		case <-ctx.Done():
			ctx.Debugf("%s: Cancelled with %s left", target.Target, time.Until(deadline))
		}
		return nil
	}

	return teststeps.ForEachTargetWithResume(ctx, ch, resumeState, 1, fn)
}
