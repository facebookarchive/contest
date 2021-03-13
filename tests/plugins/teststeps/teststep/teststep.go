// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststep

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

var Name = "Test"

const (
	// A comma-delimited list of target IDs to fail on.
	FailTargetsParam = "FailTargets"
	// Alternatively, fail this percentage of targets at random.
	FailPctParam = "FailPct"
	// A comma-delimited list of target IDs to delay and by how much, ID=delay_ms.
	DelayTargetsParam = "DelayTargets"
)

const (
	StartedEvent      = event.Name("TestStartedEvent")
	FinishedEvent     = event.Name("TestFinishedEvent")
	FailedEvent       = event.Name("TestFailedEvent")
	StepRunningEvent  = event.Name("TestStepRunningEvent")
	StepFinishedEvent = event.Name("TestStepFinishedEvent")
)

var Events = []event.Name{StartedEvent, FinishedEvent, FailedEvent, StepRunningEvent, StepFinishedEvent}

type Step struct {
	failPct      int64
	failTargets  map[string]bool
	delayTargets map[string]time.Duration
}

// Name returns the name of the Step
func (ts Step) Name() string {
	return Name
}

func (ts *Step) shouldFail(t *target.Target, params test.TestStepParameters) bool {
	if ts.failTargets[t.ID] {
		return true
	}
	if ts.failPct > 0 {
		roll := rand.Int63n(101)
		return (roll <= ts.failPct)
	}
	return false
}

// Run executes the example step.
func (ts *Step) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	f := func(ctx xcontext.Context, target *target.Target) error {
		// Sleep to ensure TargetIn fires first. This simplifies test assertions.
		time.Sleep(20 * time.Millisecond)
		if err := ev.Emit(testevent.Data{EventName: StartedEvent, Target: target, Payload: nil}); err != nil {
			return fmt.Errorf("failed to emit start event: %v", err)
		}
		delay := ts.delayTargets[target.ID]
		if delay == 0 {
			delay = ts.delayTargets["*"]
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return xcontext.ErrCanceled
		}
		if ts.shouldFail(target, params) {
			if err := ev.Emit(testevent.Data{EventName: FailedEvent, Target: target, Payload: nil}); err != nil {
				return fmt.Errorf("failed to emit finished event: %v", err)
			}
			return fmt.Errorf("target failed")
		} else {
			if err := ev.Emit(testevent.Data{EventName: FinishedEvent, Target: target, Payload: nil}); err != nil {
				return fmt.Errorf("failed to emit failed event: %v", err)
			}
		}
		return nil
	}
	if err := ev.Emit(testevent.Data{EventName: StepRunningEvent}); err != nil {
		return fmt.Errorf("failed to emit failed event: %v", err)
	}
	res := teststeps.ForEachTarget(Name, ctx, ch, f)
	if err := ev.Emit(testevent.Data{EventName: StepFinishedEvent}); err != nil {
		return fmt.Errorf("failed to emit failed event: %v", err)
	}
	return res
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *Step) ValidateParameters(params test.TestStepParameters) error {
	targetsToFail := params.GetOne(FailTargetsParam).String()
	if len(targetsToFail) > 0 {
		for _, t := range strings.Split(targetsToFail, ",") {
			ts.failTargets[t] = true
		}
	}
	targetsToDelay := params.GetOne(DelayTargetsParam).String()
	if len(targetsToDelay) > 0 {
		for _, e := range strings.Split(targetsToDelay, ",") {
			kv := strings.Split(e, "=")
			if len(kv) != 2 {
				continue
			}
			v, err := strconv.Atoi(kv[1])
			if err != nil {
				return fmt.Errorf("invalid FailTargets: %w", err)
			}
			ts.delayTargets[kv[0]] = time.Duration(v) * time.Millisecond
		}
	}
	if params.GetOne(FailPctParam).String() != "" {
		if pct, err := params.GetInt(FailPctParam); err == nil {
			ts.failPct = pct
		} else {
			return fmt.Errorf("invalid FailPct: %w", err)
		}
	}
	return nil
}

// Resume tries to resume a previously interrupted test step. TestTestStep
// cannot resume.
func (ts *Step) Resume(ctx xcontext.Context, ch test.TestStepChannels, _ test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *Step) CanResume() bool {
	return false
}

// New initializes and returns a new TestStep.
func New() test.TestStep {
	return &Step{
		failTargets:  make(map[string]bool),
		delayTargets: make(map[string]time.Duration),
	}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}
