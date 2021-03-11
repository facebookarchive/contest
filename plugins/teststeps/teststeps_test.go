// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type data struct {
	ctx           xcontext.Context
	cancel, pause func()
	inCh, outCh   chan *target.Target
	errCh         chan cerrors.TargetError
	stepChans     test.TestStepChannels
}

func newData() data {
	ctx, pause, cancel := xcontext.New()
	inCh := make(chan *target.Target)
	outCh := make(chan *target.Target)
	errCh := make(chan cerrors.TargetError)
	return data{
		ctx:    ctx,
		cancel: cancel,
		pause:  pause,
		inCh:   inCh,
		outCh:  outCh,
		errCh:  errCh,
		stepChans: test.TestStepChannels{
			In:  inCh,
			Out: outCh,
			Err: errCh,
		},
	}
}

func TestForEachTargetOneTarget(t *testing.T) {
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Printf("Handling target %+v", tgt)
		return nil
	}
	go func() {
		d.inCh <- &target.Target{ID: "target001"}
		// signal end of input
		d.inCh <- nil
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tgt := <-d.outCh:
				log.Printf("Step for target %+v completed as expected", tgt)
			case err := <-d.errCh:
				t.Errorf("Expected no error but got one: %v", err)
			}
		}
	}()
	err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetOneTargetAllFail(t *testing.T) {
	d := newData()
	fn := func(ctx xcontext.Context, t *target.Target) error {
		log.Printf("Handling target %+v", t)
		return fmt.Errorf("error with target %+v", t)
	}
	go func() {
		d.inCh <- &target.Target{ID: "target001"}
		// signal end of input
		d.inCh <- nil
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tgt := <-d.outCh:
				t.Errorf("Step for target %+v expected to fail but completed successfully instead", tgt)
			case err := <-d.errCh:
				log.Printf("Step for target failed as expected: %v", err)
			}
		}
	}()
	err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetTenTargets(t *testing.T) {
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Printf("Handling target %+v", tgt)
		return nil
	}
	go func() {
		for i := 0; i < 10; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%00d", i)}
		}
		// signal end of input
		d.inCh <- nil
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tgt := <-d.outCh:
				log.Printf("Step for target %+v completed as expected", tgt)
			case err := <-d.errCh:
				t.Errorf("Expected no error but got one: %v", err)
			}
		}
	}()
	err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetTenTargetsAllFail(t *testing.T) {
	logging.Debug()
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Printf("Handling target %+v", tgt)
		return fmt.Errorf("error with target %+v", tgt)
	}
	go func() {
		for i := 0; i < 10; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%00d", i)}
		}
		// signal end of input
		d.inCh <- nil
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tgt := <-d.outCh:
				t.Errorf("Step for target %+v expected to fail but completed successfully instead", tgt)
			case err := <-d.errCh:
				log.Printf("Step for target failed as expected: %v", err)
			}
		}
	}()
	err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetTenTargetsOneFails(t *testing.T) {
	d := newData()
	// chosen by fair dice roll.
	// guaranteed to be random.
	failingTarget := "target004"
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Printf("Handling target %+v", tgt)
		if tgt.ID == failingTarget {
			return fmt.Errorf("error with target %+v", tgt)
		}
		return nil
	}
	go func() {
		for i := 0; i < 10; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		// signal end of input
		d.inCh <- nil
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tgt := <-d.outCh:
				if tgt.ID == failingTarget {
					t.Errorf("Step for target %+v expected to fail but completed successfully instead", tgt)
				} else {
					log.Printf("Step for target %+v completed as expected", tgt)
				}
			case err := <-d.errCh:
				if err.Target.ID == failingTarget {
					log.Printf("Step for target failed as expected: %v", err)
				} else {
					t.Errorf("Expected no error but got one: %v", err)
				}
			}
		}
	}()
	err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

// TestForEachTargetTenTargetsParallelism checks if we didn't break the parallelism of
// ForEachTarget. It passes 10 targets and a function that takes 1 second for each
// target, so the whole process should not take more than ~1s if properly parallelized.
// I am using a deadline of 3s to give it some margin, knowing that if it is sequential
// it will take ~10s.
func TestForEachTargetTenTargetsParallelism(t *testing.T) {
	logging.Debug()
	sleepTime := time.Second
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Printf("Handling target %+v", tgt)
		select {
		case <-ctx.Done():
			log.Printf("target %+v cancelled", tgt)
		case <-ctx.Paused():
			log.Printf("target %+v paused", tgt)
		case <-time.After(sleepTime):
			log.Printf("target %+v processed", tgt)
		}
		return nil
	}

	numTargets := 10
	go func() {
		for i := 0; i < numTargets; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		// signal end of input
		d.inCh <- nil
	}()

	deadlineExceeded := false
	var targetError error = nil
	targetsRemain := numTargets
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		// try to cancel ForEachTarget in case it's still running
		defer d.cancel()
		defer wg.Done()

		maxWaitTime := sleepTime * 3
		deadline := time.Now().Add(maxWaitTime)
		log.Printf("Setting deadline to now+%s", maxWaitTime)

		for {
			select {
			case tgt := <-d.outCh:
				targetsRemain--
				log.Printf("Step for target completed successfully as expected: %v", tgt)
				if targetsRemain == 0 {
					log.Print("All tergates processed")
					return
				}
			case err := <-d.errCh:
				log.Printf("Step for target %+v expected to completed successfully but fail instead", err)
				targetError = err.Err
				return
			case <-time.After(time.Until(deadline)):
				deadlineExceeded = true
				log.Printf("Deadline exceeded")
				return
			}
		}
	}()

	err := ForEachTarget("test_parallel", d.ctx, d.stepChans, fn)

	wg.Wait() //wait for receiver

	if deadlineExceeded {
		t.Fatal("wait deadline exceeded, it's possible that parallelization is not working anymore")
	}
	require.NoError(t, targetError)
	require.NoError(t, err)
	assert.Equal(t, 0, targetsRemain)
}

func TestForEachTargetCancelSignalPropagation(t *testing.T) {
	logging.Debug()
	sleepTime := time.Second * 5
	numTargets := 10
	var canceledTargets int32
	d := newData()

	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Printf("Handling target %+v", tgt)
		select {
		case <-ctx.Done():
			log.Printf("target %+v caneled", tgt)
			atomic.AddInt32(&canceledTargets, 1)
		case <-ctx.Paused():
			log.Printf("target %+v paused", tgt)
		case <-time.After(sleepTime):
			log.Printf("target %+v processed", tgt)
		}
		return nil
	}

	go func() {
		for i := 0; i < numTargets; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		d.inCh <- nil
	}()

	go func() {
		time.Sleep(sleepTime / 3)
		d.cancel()
	}()

	err := ForEachTarget("test_cancelation", d.ctx, d.stepChans, fn)
	require.NoError(t, err)

	assert.Equal(t, int32(numTargets), canceledTargets)
}

func TestForEachTargetCancelBeforeInputChannelClosed(t *testing.T) {
	logging.Debug()
	sleepTime := time.Second * 5
	numTargets := 10
	var canceledTargets int32
	d := newData()

	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Printf("Handling target %+v", tgt)
		select {
		case <-ctx.Done():
			log.Printf("target %+v cancelled", tgt)
			atomic.AddInt32(&canceledTargets, 1)
		case <-ctx.Paused():
			log.Printf("target %+v paused", tgt)
		case <-time.After(sleepTime):
			log.Printf("target %+v processed", tgt)
		}
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for i := 0; i < numTargets; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		wg.Wait() //Don't close the input channel until ForEachTarget returned
	}()

	go func() {
		time.Sleep(sleepTime / 3)
		d.cancel()
	}()

	err := ForEachTarget("test_cancelation", d.ctx, d.stepChans, fn)
	require.NoError(t, err)

	wg.Done()
	assert.Equal(t, int32(numTargets), canceledTargets)
}
