// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// PerTargetFunc is a function type that is called on each target by the
// ForEachTarget function below.
type PerTargetFunc func(ctx xcontext.Context, target *target.Target) error

// ForEachTarget is a simple helper to write plugins that apply a given PerTargetFunc
// independenly to each target. This helper handles routing through the in/out channels,
// and forwards cancel/pause signals to the PerTargetFunc.
// Note this helper does NOT saving state and pausing a test step, so it is only suited for
// short-running tests or for environments that don't use job resumption.
// Use ForEachTargetWithResume below for a similar helper with full resumption support.
// This function wraps the logic that handles target routing through the in/out
// The implementation of the per-target function is responsible for
// reacting to cancel/pause signals and return quickly.
func ForEachTarget(pluginName string, ctx xcontext.Context, ch test.TestStepChannels, f PerTargetFunc) (json.RawMessage, error) {
	reportTarget := func(t *target.Target, err error) {
		if err != nil {
			ctx.Errorf("%s: ForEachTarget: failed to apply test step function on target %s: %v", pluginName, t, err)
		} else {
			ctx.Debugf("%s: ForEachTarget: target %s completed successfully", pluginName, t)
		}
		select {
		case ch.Out <- test.TestStepResult{Target: t, Err: err}:
		case <-ctx.Done():
			ctx.Debugf("%s: ForEachTarget: received cancellation signal while reporting result", pluginName)
		}
	}

	var wg sync.WaitGroup
	func() {
		for {
			select {
			case tgt, ok := <-ch.In:
				if !ok {
					ctx.Debugf("%s: ForEachTarget: all targets have been received", pluginName)
					return
				}
				ctx.Debugf("%s: ForEachTarget: received target %s", pluginName, tgt)
				wg.Add(1)
				go func() {
					defer wg.Done()

					err := f(ctx, tgt)
					reportTarget(tgt, err)
				}()
			case <-ctx.Done():
				ctx.Debugf("%s: ForEachTarget: incoming loop canceled", pluginName)
				return
			}
		}
	}()
	wg.Wait()
	return nil, nil
}

// MarshalState serializes the provided state struct as JSON.
// It sets the Version field to the specified value.
func MarshalState(state interface{}, version int) (json.RawMessage, error) {
	{ // Set the version.
		vs := reflect.Indirect(reflect.ValueOf(state))
		if vs.Kind() != reflect.Struct {
			return nil, fmt.Errorf("state must be a struct")
		}
		vf := vs.FieldByName("Version")
		if vf.Kind() == 0 {
			return nil, fmt.Errorf("no Version field in struct")
		}
		vf.SetInt(int64(version))
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	return data, xcontext.ErrPaused
}

// TargetWithData holds a step target and the pause/resumption data for it
// Each per-target function gets this passed in and can store any data
// required to resume in data.
type TargetWithData struct {
	Target *target.Target
	Data   json.RawMessage
}

// PerTargetWithResumeFunc is the function that is called per target by ForEachTargetWithResume
// It must obey the context and quickly return on cancellation and pause signals.
// Functions can modify target and store any data required for resumption in target.data.
type PerTargetWithResumeFunc func(ctx xcontext.Context, target *TargetWithData) error

// parallelTargetsState is the internal state of ForEachTargetWithResume.
type parallelTargetsState struct {
	Version int               `json:"V"`
	Targets []*TargetWithData `json:"TWD,omitempty"`
}

// ForEachTargetWithResume is a helper to write plugins that support job resumption.
// This helper is for plugins that want to apply a single function to all targets, independently.
// This helper calls the supplied PerTargetWithResumeFunc immediately for each target received,
// in a separate goroutine. When the function returns, the target is sent to the output channels so it can
// run through the next test step.
// This helper directly accepts the resumeState from the Run method of the TestStep interface, and the
// return value can directly be passed back to the framework.
// The helper automatically manages data returned on pause and makes sure the function is called again
// with the same data on job resumption. The helper will not call functions again that succeeded or failed
// before the pause signal was received.
// The supplied PerTargetWithResumeFunc must react to pause and cancellation signals as normal.
func ForEachTargetWithResume(ctx xcontext.Context, ch test.TestStepChannels, resumeState json.RawMessage, currentStepStateVersion int, f PerTargetWithResumeFunc) (json.RawMessage, error) {
	var ss parallelTargetsState

	// Parse resume state, if any.
	if len(resumeState) > 0 {
		if err := json.Unmarshal(resumeState, &ss); err != nil {
			return nil, fmt.Errorf("invalid resume state: %w", err)
		}
		if ss.Version != currentStepStateVersion {
			return nil, fmt.Errorf("incompatible resume state: want %d, got %d", currentStepStateVersion, ss.Version)
		}
	}

	var wg sync.WaitGroup
	pauseStates := make(chan *TargetWithData)

	handleTarget := func(tgt2 *TargetWithData) {
		defer wg.Done()

		err := f(ctx, tgt2)
		switch err {
		case xcontext.ErrCanceled:
			// nothing to do for failed
		case xcontext.ErrPaused:
			select {
			case pauseStates <- tgt2:
			case <-ctx.Done():
				ctx.Debugf("ForEachTargetWithResume: received cancellation signal while pausing")
			}
		default:
			// nil or error
			if err != nil {
				ctx.Errorf("ForEachTargetWithResume: failed to apply test step function on target %s: %v", tgt2.Target.ID, err)
			} else {
				ctx.Debugf("ForEachTargetWithResume: target %s completed successfully", tgt2.Target.ID)
			}
			select {
			case ch.Out <- test.TestStepResult{Target: tgt2.Target, Err: err}:
			case <-ctx.Done():
				ctx.Debugf("ForEachTargetWithResume: received cancellation signal while reporting result")
			}
		}
	}

	// restart paused targets
	for _, state := range ss.Targets {
		ctx.Debugf("ForEachTargetWithResume: resuming target %s", state.Target.ID)
		wg.Add(1)
		go handleTarget(state)
	}
	// delete info about running targets
	ss.Targets = nil

	var err error
mainloop:
	for {
		select {
		// no need to check for pause here, pausing closes the channel
		case tgt, ok := <-ch.In:
			if !ok {
				break mainloop
			}
			ctx.Debugf("ForEachTargetWithResume: received target %s", tgt)
			wg.Add(1)
			go handleTarget(&TargetWithData{Target: tgt})
		case <-ctx.Done():
			ctx.Debugf("ForEachTargetWithResume: canceled, terminating")
			err = xcontext.ErrCanceled
			break mainloop
		}
	}

	// close pauseStates to signal all handlers are done
	go func() {
		wg.Wait()
		close(pauseStates)
	}()

	for ps := range pauseStates {
		ss.Targets = append(ss.Targets, ps)
	}

	// wrap up
	if !ctx.IsSignaledWith(xcontext.ErrPaused) && len(ss.Targets) > 0 {
		return nil, fmt.Errorf("ForEachTargetWithResume: some target functions paused, but no pause signal received: %v ", ss.Targets)
	}
	if ctx.IsSignaledWith(xcontext.ErrPaused) {
		return MarshalState(&ss, currentStepStateVersion)
	}
	return nil, err
}
