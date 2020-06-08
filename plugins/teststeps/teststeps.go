// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

var log = logging.GetLogger("plugins/teststeps")

// PerTargetFunc is a function type that is called on each target by the
// ForEachTarget function below.
type PerTargetFunc func(cancel, pause <-chan struct{}, target *target.Target) error

// ForEachTarget is a facility provided to simplify plugin implemtations. This
// function wraps the logic that handles target routing through the in/out/err
// test step channels, and also handles cancellation and pausing.
// The user of this function has to pass the cancel, pause, and test step
// channels as received in the Run method of the plugin, and additionally
// provide an implementation of a per-target function that will be called on
// each target. The implementation of the per-target function is responsible for
// handling internal cancellation and pausing.
func ForEachTarget(pluginName string, cancel, pause <-chan struct{}, ch test.StepChannels, f PerTargetFunc) error {
	for {
		select {
		case t := <-ch.In:
			if t == nil {
				// no more targets incoming
				return nil
			}
			log.Debugf("%s: ForEachTarget: received target %s", pluginName, t)
			errCh := make(chan error)
			go func() {
				log.Debugf("%s: ForEachTarget: calling function on target %s", pluginName, t)
				errCh <- f(cancel, pause, t)
			}()

			select {
			case err := <-errCh:
				result := &target.Result{Target: t}
				if err != nil {
					result.Err = err
				}
				select {
				case ch.Out <- result:
					log.Debugf("%s: ForEachTarget: target %s completed successfully", pluginName, t)
				case <-cancel:
					log.Debugf("%s: ForEachTarget: received cancellation signal", pluginName)
					return nil
				case <-pause:
					log.Debugf("%s: ForEachTarget: received pausing signal", pluginName)
					return nil
				}

			case <-cancel:
				return nil
			case <-pause:
				return nil
			}
		case <-cancel:
			return nil
		case <-pause:
			return nil
		}
	}
}
