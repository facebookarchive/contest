// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/stretchr/testify/require"
)

func TestWaitForFirstTarget(t *testing.T) {
	t.Run("100targets", func(t *testing.T) {
		ch0 := make(chan *target.Target)
		ch1, onFirstTargetChan, onNoTargetsChan := waitForFirstTarget(ch0, nil, nil)

		var wgBeforeFirstTarget, wgAfterSecondTarget sync.WaitGroup
		wgBeforeFirstTarget.Add(1)
		wgAfterSecondTarget.Add(1)

		go func() {
			wgBeforeFirstTarget.Wait()
			for i := 0; i < 100; i++ {
				ch0 <- &target.Target{
					ID: fmt.Sprintf("%d", i),
				}
				if i == 1 {
					// to avoid race conditions in this test we verify
					// if there's a "onFirstTarget" signal after the second
					// target
					wgAfterSecondTarget.Done()
				}
			}
		}()

		runtime.Gosched()
		select {
		case <-onFirstTargetChan:
			t.Fatal("should not happen")
		case <-onNoTargetsChan:
			t.Fatal("should not happen")
		default:
		}

		wgBeforeFirstTarget.Done()

		var wgReader sync.WaitGroup
		wgReader.Add(1)
		go func() {
			defer wgReader.Done()
			// Checking quantity and order
			for i := 0; i < 100; i++ {
				_target := <-ch1
				require.Equal(t, &target.Target{
					ID: fmt.Sprintf("%d", i),
				}, _target)
			}
		}()

		wgAfterSecondTarget.Wait()

		select {
		case <-onFirstTargetChan:
		case <-onNoTargetsChan:
			t.Fatal("should not happen")
		default:
			t.Fatal("should not happen") // see wgAfterSecondTarget.Wait() above
		}

		wgReader.Wait()

		runtime.Gosched()
		require.Len(t, ch1, 0)
	})

	t.Run("no_target", func(t *testing.T) {
		ch0 := make(chan *target.Target)
		ch1, onFirstTargetChan, onNoTargetsChan := waitForFirstTarget(ch0, nil, nil)

		runtime.Gosched()
		select {
		case <-onFirstTargetChan:
			t.Fatal("should not happen")
		case <-onNoTargetsChan:
			t.Fatal("should not happen")
		default:
		}

		close(ch0)

		select {
		case <-onFirstTargetChan:
			t.Fatal("should not happen")
		case <-onNoTargetsChan:
		}

		runtime.Gosched()
		require.Len(t, ch1, 0)
		_, isOpened := <-ch1
		require.False(t, isOpened)
	})

	t.Run("cancel", func(t *testing.T) {
		cancelCh := make(chan struct{})
		ch0 := make(chan *target.Target)
		ch1, onFirstTargetChan, onNoTargetsChan := waitForFirstTarget(ch0, cancelCh, nil)

		runtime.Gosched()
		select {
		case <-onFirstTargetChan:
			t.Fatal("should not happen") // see wgBeforeFirstTarget.Wait() above
		case <-onNoTargetsChan:
			t.Fatal("should not happen")
		default:
		}

		close(cancelCh)

		runtime.Gosched()
		select {
		case <-onFirstTargetChan:
			t.Fatal("should not happen")
		case <-onNoTargetsChan:
			t.Fatal("should not happen")
		default:
		}

		require.Len(t, ch1, 0)
		select {
		case <-ch1:
			t.Fatal("should not happen")
		default:
		}
	})
}
