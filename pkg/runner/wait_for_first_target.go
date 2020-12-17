// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"context"
	"github.com/facebookincubator/contest/pkg/target"
)

// waitForFirstTarget copies targets from `in` to `out` and also
// signals on first target or if `in` is closed and there was no targets
// at all.
//
// To solve the initial problem there're two approaches:
// * Just wait for a target in `stepCh.stepIn` and then insert
// it back to the channel.
// * Create substitute `stepCh.stepIn` with a new channel and just
// copy all targets from original `stepCh.stepIn` to this new channel.
// And we may detect when we receive the first target in the original
// channel.
//
// The first approach is much simpler, but the second preserves
// the order of targets (which is handy and not misleading
// while reading logs). Here we implement the second one:
func waitForFirstTarget(
	ctx context.Context,
	in <-chan *target.Target,
) (out chan *target.Target, onFirstTarget, onNoTargets <-chan struct{}) {
	onFirstTargetCh := make(chan struct{})
	onNoTargetsCh := make(chan struct{})
	onFirstTarget, onNoTargets = onFirstTargetCh, onNoTargetsCh

	out = make(chan *target.Target)
	go func() {
		select {
		case t, ok := <-in:
			if !ok {
				close(out)
				close(onNoTargetsCh)
				return
			}
			close(onFirstTargetCh)
			out <- t
		case <-ctx.Done():
			return
		}

		for t := range in {
			out <- t
		}
		close(out)
	}()

	return
}
