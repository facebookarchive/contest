// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"sync/atomic"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
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
	type tgtErr struct {
		target *target.Target
		err    error
	}
	tgtErrCh := make(chan tgtErr)
	tgtInFlight := int32(0)
	noMoreTargetsCh := make(chan struct{})

	go func(tgtInFlight *int32) {
		defer func() {
			log.Debugf("%s: ForEachTarget: exiting incoming loop, targets inflight %d", pluginName, atomic.LoadInt32(tgtInFlight))
		}()
		for {
			select {
			case tgt := <-ch.In:
				log.Debugf("%s: ForEachTarget: received target %s", pluginName, tgt)
				if tgt == nil {
					log.Debugf("%s: ForEachTarget: all targets have been received", pluginName)
					noMoreTargetsCh <- struct{}{}
					return
				}

				go func() {
					tgtErrCh <- tgtErr{target: tgt, err: f(cancel, pause, tgt)}
				}()
				atomic.AddInt32(tgtInFlight, 1)
			case <-cancel:
				log.Debugf("%s: ForEachTarget: incoming loop canceled", pluginName)
				return
			case <-pause:
				log.Debugf("%s: ForEachTarget: incoming loop paused", pluginName)
				return
			}

		}
	}(&tgtInFlight)

	noMoreTargets := false
	reportResults := true
	defer func() {
		log.Debugf("%s: ForEachTarget: exiting outgoing loop, targets inflight %d, the last target received %v", pluginName, atomic.LoadInt32(&tgtInFlight), noMoreTargets)
	}()
	for {
		select {
		case te := <-tgtErrCh:
			atomic.AddInt32(&tgtInFlight, -1)
			if reportResults {
				if te.err != nil {
					log.Errorf("%s: ForEachTarget: failed to apply test step function on target %s: %v", pluginName, te.target, te.err)
					select {
					case ch.Err <- cerrors.TargetError{Target: te.target, Err: te.err}:
					case <-cancel:
						log.Debugf("%s: ForEachTarget: received cancellation signal while reporting error", pluginName)
						reportResults = false
					case <-pause:
						log.Debugf("%s: ForEachTarget: received pausing signal while reporting error", pluginName)
						reportResults = false
					}
				} else {
					log.Debugf("%s: ForEachTarget: target %s completed successfully", pluginName, te.target)
					select {
					case ch.Out <- te.target:
					case <-cancel:
						log.Debugf("%s: ForEachTarget: received cancellation signal while reporting success", pluginName)
						reportResults = false
					case <-pause:
						log.Debugf("%s: ForEachTarget: received pausing signal while reporting success", pluginName)
						reportResults = false
					}
				}
			} else {
				log.Debugf("%s: ForEachTarget: the result is ignored due to cancellation: %v", pluginName, te)
			}
			if atomic.LoadInt32(&tgtInFlight) == 0 && (noMoreTargets || !reportResults) {
				return nil
			}
		case <-noMoreTargetsCh:
			log.Debugf("%s: ForEachTarget: received noMoreTargets signal", pluginName)
			if atomic.LoadInt32(&tgtInFlight) == 0 {
				return nil
			}
			noMoreTargets = true
		case <-cancel:
			log.Debugf("%s: ForEachTarget: received cancellation signal while waiting for results", pluginName)
			reportResults = false
		case <-pause:
			log.Debugf("%s: ForEachTarget: received pausing signal while waiting for results", pluginName)
			reportResults = false
		}
	}
}
