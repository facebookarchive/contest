// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"sync"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

var log = logging.GetLogger("plugins/teststeps")

// PerTargetFunc is a function type that is called on each target by the
// ForEachTarget function below.
type PerTargetFunc func(ctx statectx.Context, target *target.Target) error

// ForEachTarget is a facility provided to simplify plugin implementations. This
// function wraps the logic that handles target routing through the in/out/err
// test step channels, and also handles cancellation and pausing.
// The user of this function has to pass the cancel, pause, and test step
// channels as received in the Run method of the plugin, and additionally
// provide an implementation of a per-target function that will be called on
// each target. The implementation of the per-target function is responsible for
// handling internal cancellation and pausing.
func ForEachTarget(pluginName string, ctx statectx.Context, ch test.TestStepChannels, f PerTargetFunc) error {
	reportTarget := func(t *target.Target, err error) {
		if err != nil {
			log.Errorf("%s: ForEachTarget: failed to apply test step function on target %s: %v", pluginName, t, err)
			select {
			case ch.Err <- cerrors.TargetError{Target: t, Err: err}:
			case <-ctx.PausedOrDone():
				log.Debugf("%s: ForEachTarget: received cancellation/pause signal while reporting error", pluginName)
			}
		} else {
			log.Debugf("%s: ForEachTarget: target %s completed successfully", pluginName, t)
			select {
			case ch.Out <- t:
			case <-ctx.PausedOrDone():
				log.Debugf("%s: ForEachTarget: received cancellation/pause signal while reporting success", pluginName)
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

					err := f(ctx, tgt)
					reportTarget(tgt, err)
				}()
			case <-ctx.Done():
				log.Debugf("%s: ForEachTarget: incoming loop canceled", pluginName)
				return
			case <-ctx.Paused():
				log.Debugf("%s: ForEachTarget: incoming loop paused", pluginName)
				return
			}
		}
	}()
	wg.Wait()
	return nil
}
