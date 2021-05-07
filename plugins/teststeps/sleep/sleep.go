// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package sleep

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
var Name = "Sleep"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

// sleepStep implements an echo-style printing plugin.
type sleepStep struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	inFlight map[*target.Target]time.Time
}

// New initializes and returns a new EchoStep. It implements the TestStepFactory
// interface.
func New() test.TestStep {
	return &sleepStep{
		inFlight: make(map[*target.Target]time.Time),
	}
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

type targetState struct {
	Target     *target.Target `json:"T"`
	DeadlineMS int64          `json:"D"`
}

const currentStateVersion = 1

type sleepStepState struct {
	Version  int `json:"V"`
	InFlight []targetState
}

func (ss *sleepStep) loadState(ctx xcontext.Context, resumeState json.RawMessage, out chan<- test.TestStepResult) error {
	if len(resumeState) == 0 {
		return nil
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	var sss sleepStepState
	if err := json.Unmarshal(resumeState, &sss); err != nil {
		return fmt.Errorf("invalid resume state: %w", err)
	}
	if sss.Version != currentStateVersion {
		return fmt.Errorf("incompatible resume state (want %d, got %d)", currentStateVersion, sss.Version)
	}
	now := time.Now()
	for _, e := range sss.InFlight {
		ss.wg.Add(1)
		deadline := time.Unix(e.DeadlineMS/1000, (e.DeadlineMS%1000)*1000000)
		go ss.doSleep(ctx, e.Target, deadline.Sub(now), out)
	}
	return nil
}

func (ss *sleepStep) doSleep(ctx xcontext.Context, t *target.Target, dur time.Duration, out chan<- test.TestStepResult) {
	deadline := time.Now().Add(dur)
	ss.mu.Lock()
	ss.inFlight[t] = deadline
	ss.mu.Unlock()
	ctx.Debugf("%s: Sleeping for %s", t, dur)
	select {
	case <-time.After(dur):
		select {
		case out <- test.TestStepResult{Target: t, Err: nil}:
			ss.mu.Lock()
			delete(ss.inFlight, t)
			ss.mu.Unlock()
		case <-ctx.Done():
		}
	case <-ctx.Until(xcontext.ErrPaused):
		ctx.Debugf("%s: Paused with %s left", t, time.Until(deadline))
	case <-ctx.Done():
		ctx.Debugf("%s: Cancelled with %s left", t, time.Until(deadline))
	}
	ss.wg.Done()
}

// Run executes the step
func (ss *sleepStep) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	dur, err := getDuration(params)
	if err != nil {
		return nil, err
	}
	if err := ss.loadState(ctx, resumeState, ch.Out); err != nil {
		return nil, err
	}
loop:
	for {
		select {
		case t, ok := <-ch.In:
			if ok {
				ss.wg.Add(1)
				go ss.doSleep(ctx, t, dur, ch.Out)
			} else {
				break loop
			}
		case <-ctx.Done():
			break loop
		}
	}
	ss.wg.Wait()
	select {
	case <-ctx.Until(xcontext.ErrPaused):
		sss := sleepStepState{
			Version: currentStateVersion,
		}
		ss.mu.Lock()
		for t, deadline := range ss.inFlight {
			deadlineMS := deadline.UnixNano() / 1000000
			sss.InFlight = append(sss.InFlight, targetState{
				Target:     t,
				DeadlineMS: deadlineMS,
			})
		}
		ss.mu.Unlock()
		resumeState, err := json.Marshal(&sss)
		if err != nil {
			return nil, err
		}
		return resumeState, xcontext.ErrPaused
	default:
	}
	return nil, nil
}
