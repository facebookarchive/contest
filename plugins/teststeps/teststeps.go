// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"context"
	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"sync"
)

var log = logging.GetLogger("plugins/teststeps")

// PerTargetFunc is a function type that is called on each target by the
// ForEachTarget function below.
type PerTargetFunc func(cancel, pause <-chan struct{}, target *target.Target) error

// ForEachTarget is a facility provided to simplify plugin implementations. This
// function wraps the logic that handles target routing through the in/out/err
// test step channels, and also handles cancellation and pausing.
// The user of this function has to pass the cancel, pause, and test step
// channels as received in the Run method of the plugin, and additionally
// provide an implementation of a per-target function that will be called on
// each target. The implementation of the per-target function is responsible for
// handling internal cancellation and pausing.
func ForEachTarget(pluginName string, cancel, pause <-chan struct{}, ch test.TestStepChannels, f PerTargetFunc) error {
	// Temporary solution
	ctx, cancelCtx := combineChannelsToContext(cancel, pause)
	defer cancelCtx()

	reportTarget := func(t *target.Target, err error) {
		if err != nil {
			log.Errorf("%s: ForEachTarget: failed to apply test step function on target %s: %v", pluginName, t, err)
			select {
			case ch.Err <- cerrors.TargetError{Target: t, Err: err}:
			case <-ctx.Done():
				log.Debugf("%s: ForEachTarget: received cancellation/pause signal while reporting error", pluginName)
			}
		} else {
			log.Debugf("%s: ForEachTarget: target %s completed successfully", pluginName, t)
			select {
			case ch.Out <- t:
			case <-ctx.Done():
				log.Debugf("%s: ForEachTarget: received cancellation/pause signal while reporting error", pluginName)
			}
		}
	}

	var wg sync.WaitGroup
	func() {
		for {
			select {
			case tgt := <-ch.In:
				log.Debugf("%s: ForEachTarget: received target %s", pluginName, tgt)
				if tgt == nil {
					log.Debugf("%s: ForEachTarget: all targets have been received", pluginName)
					return
				}

				wg.Add(1)
				go func() {
					defer wg.Done()

					err := f(cancel, pause, tgt)
					reportTarget(tgt, err)
				}()
			case <-ctx.Done():
				log.Debugf("launching new targets has been cancelled")
				return
			}
		}
	}()
	wg.Wait()
	return nil
}

func combineChannelsToContext(cancel, pause <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		select {
		case <-cancel:
			cancelFunc()
		case <-pause:
			cancelFunc()
		case <-ctx.Done():
		}
	}()
	return ctx, cancelFunc
}
