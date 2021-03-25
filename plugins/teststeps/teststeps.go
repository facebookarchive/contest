// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"sync"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// PerTargetFunc is a function type that is called on each target by the
// ForEachTarget function below.
type PerTargetFunc func(ctx xcontext.Context, target *target.Target) error

// ForEachTarget is a facility provided to simplify plugin implementations. This
// function wraps the logic that handles target routing through the in/out/err
// test step channels, and also handles cancellation and pausing.
// The user of this function has to pass the cancel, pause, and test step
// channels as received in the Run method of the plugin, and additionally
// provide an implementation of a per-target function that will be called on
// each target. The implementation of the per-target function is responsible for
// handling internal cancellation and pausing.
func ForEachTarget(pluginName string, ctx xcontext.Context, ch test.TestStepChannels, f PerTargetFunc) error {
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
	return nil
}
