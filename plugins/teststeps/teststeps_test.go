// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package teststeps

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"

	"github.com/facebookincubator/contest/tests/common/goroutine_leak_check"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type data struct {
	ctx           xcontext.Context
	cancel, pause func()
	inCh          chan *target.Target
	outCh         chan test.TestStepResult
	stepChans     test.TestStepChannels
}

func newData() data {
	ctx, pause := xcontext.WithNotify(nil, xcontext.ErrPaused)
	ctx, cancel := xcontext.WithCancel(ctx)
	inCh := make(chan *target.Target)
	outCh := make(chan test.TestStepResult)
	return data{
		ctx:    ctx,
		cancel: cancel,
		pause:  pause,
		inCh:   inCh,
		outCh:  outCh,
		stepChans: test.TestStepChannels{
			In:  inCh,
			Out: outCh,
		},
	}
}

func TestForEachTargetOneTarget(t *testing.T) {
	ctx := logrusctx.NewContext(logger.LevelDebug)
	log := ctx.Logger()
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		log.Debugf("Handling target %s", tgt)
		return nil
	}
	go func() {
		d.inCh <- &target.Target{ID: "target001"}
		close(d.inCh)
	}()
	ctx, cancel := xcontext.WithCancel(ctx)
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case res := <-d.outCh:
				if res.Err == nil {
					log.Debugf("Step for target %s completed as expected", res.Target)
				} else {
					t.Errorf("Expected no error but got one: %v", res.Err)
				}
			}
		}
	}()
	_, err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetOneTargetAllFail(t *testing.T) {
	ctx := logrusctx.NewContext(logger.LevelDebug)
	log := ctx.Logger()
	d := newData()
	fn := func(ctx xcontext.Context, t *target.Target) error {
		log.Debugf("Handling target %s", t)
		return fmt.Errorf("error with target %s", t)
	}
	go func() {
		d.inCh <- &target.Target{ID: "target001"}
		close(d.inCh)
	}()
	ctx, cancel := xcontext.WithCancel(ctx)
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case res := <-d.outCh:
				if res.Err == nil {
					t.Errorf("Step for target %s expected to fail but completed successfully instead", res.Target)
				} else {
					log.Debugf("Step for target failed as expected: %v", res.Err)
				}
			}
		}
	}()
	_, err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetTenTargets(t *testing.T) {
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		ctx.Debugf("Handling target %s", tgt)
		return nil
	}
	go func() {
		for i := 0; i < 10; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%00d", i)}
		}
		close(d.inCh)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case res := <-d.outCh:
				if res.Err == nil {
					d.ctx.Debugf("Step for target %s completed as expected", res.Target)
				} else {
					t.Errorf("Expected no error but got one: %v", res.Err)
				}
			}
		}
	}()
	_, err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetTenTargetsAllFail(t *testing.T) {
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		d.ctx.Debugf("Handling target %s", tgt)
		return fmt.Errorf("error with target %s", tgt)
	}
	go func() {
		for i := 0; i < 10; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%00d", i)}
		}
		close(d.inCh)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case res := <-d.outCh:
				if res.Err == nil {
					t.Errorf("Step for target %s expected to fail but completed successfully instead", res.Target)
				} else {
					d.ctx.Debugf("Step for target failed as expected: %v", res.Err)
				}
			}
		}
	}()
	_, err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

func TestForEachTargetTenTargetsOneFails(t *testing.T) {
	d := newData()
	// chosen by fair dice roll.
	// guaranteed to be random.
	failingTarget := "target004"
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		d.ctx.Debugf("Handling target %s", tgt)
		if tgt.ID == failingTarget {
			return fmt.Errorf("error with target %s", tgt)
		}
		return nil
	}
	go func() {
		for i := 0; i < 10; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		close(d.inCh)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case res := <-d.outCh:
				if res.Err == nil {
					if res.Target.ID == failingTarget {
						t.Errorf("Step for target %s expected to fail but completed successfully instead", res.Target)
					} else {
						d.ctx.Debugf("Step for target %s completed as expected", res.Target)
					}
				} else {
					if res.Target.ID == failingTarget {
						d.ctx.Debugf("Step for target %s failed as expected: %v", res.Target, res.Err)
					} else {
						t.Errorf("Expected no error for %s but got one: %v", res.Target, res.Err)
					}
				}
			}
		}
	}()
	_, err := ForEachTarget("test_one_target ", d.ctx, d.stepChans, fn)
	require.NoError(t, err)
}

// TestForEachTargetTenTargetsParallelism checks if we didn't break the parallelism of
// ForEachTarget. It passes 10 targets and a function that takes 1 second for each
// target, so the whole process should not take more than ~1s if properly parallelized.
// I am using a deadline of 3s to give it some margin, knowing that if it is sequential
// it will take ~10s.
func TestForEachTargetTenTargetsParallelism(t *testing.T) {
	sleepTime := 300 * time.Millisecond
	d := newData()
	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		d.ctx.Debugf("Handling target %s", tgt)
		select {
		case <-ctx.Done():
			d.ctx.Debugf("target %s cancelled", tgt)
		case <-ctx.Until(xcontext.ErrPaused):
			d.ctx.Debugf("target %s paused", tgt)
		case <-time.After(sleepTime):
			d.ctx.Debugf("target %s processed", tgt)
		}
		return nil
	}

	numTargets := 10
	go func() {
		for i := 0; i < numTargets; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		close(d.inCh)
	}()

	deadlineExceeded := false
	var targetError error
	targetsRemain := numTargets
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		// try to cancel ForEachTarget in case it's still running
		defer d.cancel()
		defer wg.Done()

		maxWaitTime := sleepTime * 3
		deadline := time.Now().Add(maxWaitTime)
		d.ctx.Debugf("Setting deadline to now+%s", maxWaitTime)

		for {
			select {
			case res := <-d.outCh:
				targetsRemain--
				if res.Err == nil {
					d.ctx.Debugf("Step for target %s completed successfully as expected", res.Target)
				} else {
					d.ctx.Debugf("Step for target %s expected to completed successfully but failed instead", res.Target, res.Err)
					targetError = res.Err
				}
				if targetsRemain == 0 {
					d.ctx.Debugf("All targets processed")
					return
				}
			case <-time.After(time.Until(deadline)):
				deadlineExceeded = true
				d.ctx.Debugf("Deadline exceeded")
				return
			}
		}
	}()

	_, err := ForEachTarget("test_parallel", d.ctx, d.stepChans, fn)

	wg.Wait() //wait for receiver

	if deadlineExceeded {
		t.Fatal("wait deadline exceeded, it's possible that parallelization is not working anymore")
	}
	require.NoError(t, targetError)
	require.NoError(t, err)
	assert.Equal(t, 0, targetsRemain)
}

func TestForEachTargetCancelSignalPropagation(t *testing.T) {
	sleepTime := 300 * time.Millisecond
	numTargets := 10
	var canceledTargets int32
	d := newData()

	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		d.ctx.Debugf("Handling target %s", tgt)
		select {
		case <-ctx.Done():
			d.ctx.Debugf("target %s caneled", tgt)
			atomic.AddInt32(&canceledTargets, 1)
		case <-ctx.Until(xcontext.ErrPaused):
			d.ctx.Debugf("target %s paused", tgt)
		case <-time.After(sleepTime):
			d.ctx.Debugf("target %s processed", tgt)
		}
		return nil
	}

	go func() {
		for i := 0; i < numTargets; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		close(d.inCh)
	}()

	go func() {
		time.Sleep(sleepTime / 3)
		d.cancel()
	}()

	_, err := ForEachTarget("test_cancelation", d.ctx, d.stepChans, fn)
	require.NoError(t, err)

	assert.Equal(t, int32(numTargets), canceledTargets)
}

func TestForEachTargetCancelBeforeInputChannelClosed(t *testing.T) {
	sleepTime := 300 * time.Millisecond
	numTargets := 10
	var canceledTargets int32
	d := newData()

	fn := func(ctx xcontext.Context, tgt *target.Target) error {
		d.ctx.Debugf("Handling target %s", tgt)
		select {
		case <-ctx.Done():
			d.ctx.Debugf("target %s cancelled", tgt)
			atomic.AddInt32(&canceledTargets, 1)
		case <-ctx.Until(xcontext.ErrPaused):
			d.ctx.Debugf("target %s paused", tgt)
		case <-time.After(sleepTime):
			d.ctx.Debugf("target %s processed", tgt)
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

	_, err := ForEachTarget("test_cancelation", d.ctx, d.stepChans, fn)
	require.NoError(t, err)

	wg.Done()
	assert.Equal(t, int32(numTargets), canceledTargets)
}

func TestForEachTargetWithResumeAllReturn(t * testing.T) {
	numTargets := 10
	d := newData()

	fn := func(ctx xcontext.Context, target *TargetWithData) error {
		return nil // success
	}

	var wg sync.WaitGroup
	wg.Add(1)
	// submit all, then close
	go func() {
		for i := 0; i < numTargets; i++ {
			d.inCh <- &target.Target{ID: fmt.Sprintf("target%03d", i)}
		}
		close(d.inCh)
		wg.Done()
	}()

	wg.Add(1)
	// read all results
	go func() {
		for i := 0; i < numTargets; i++ {
			<-d.outCh
		}
		wg.Done()
	}()

	res, err := ForEachTargetWithResume(d.ctx, d.stepChans, nil, 1, fn)
	require.NoError(t, err)
	assert.Nil(t, res)
	// make sure all helpers are done
	wg.Wait()
	assert.NoError(t, goroutine_leak_check.CheckLeakedGoRoutines())
}

type simpleStepData struct {
	Foo string
}

func TestForEachTargetWithResumeAllPause(t * testing.T) {
	numTargets := 10
	targets := make([]target.Target, 10)
	for i := 0; i < numTargets; i++ {
		targets[i] = target.Target{ID: fmt.Sprintf("target%03d", i)}
	}
	d := newData()

	fn := func(ctx xcontext.Context, target *TargetWithData) error {
		stepData := simpleStepData{target.Target.ID}
		json, err := json.Marshal(&stepData)
		require.NoError(t, err)
		// block and pause
		<-ctx.Until(xcontext.ErrPaused)
		target.Data = json
		return xcontext.ErrPaused
	}
	var testingWg sync.WaitGroup

	// constantly read out channel, must not receive anything
	outDone := make(chan struct{})
	testingWg.Add(1)
	go func() {
		select {
		case res := <- d.outCh:
			assert.Fail(t, "unexpected target in out channel", res)
		case <-outDone:
			testingWg.Done()
		}
	}()

	var inputWg sync.WaitGroup
	inputWg.Add(1)
	// submit all, then close
	go func() {
		for i := 0; i < numTargets; i++ {
			d.inCh <- &targets[i]
		}
		close(d.inCh)
		inputWg.Done()
	}()

	// run helper so it accepts jobs
	testingWg.Add(1)
	go func() {
		res, err := ForEachTargetWithResume(d.ctx, d.stepChans, nil, 1, fn)
		assert.Equal(t, xcontext.ErrPaused, err)
		// inspect result
		state := parallelTargetsState{}
		assert.NoError(t, json.Unmarshal(res, &state))
		assert.Equal(t, 1, state.Version)
		assert.Equal(t, numTargets, len(state.Targets))
		targetSeen := make(map[string]*TargetWithData)
		// check all targets were returned once
		for _, twd := range state.Targets {
			_, ok := targetSeen[twd.Target.ID]
			if ok {
				assert.Fail(t, "duplicate target data in serialized resume data", twd)
			}
			targetSeen[twd.Target.ID] = twd
		}
		for i := 0; i < numTargets; i++ {
			twd, ok := targetSeen[targets[i].ID]
			assert.True(t, ok)
			// check serialized data
			stepData := simpleStepData{}
			assert.NoError(t, json.Unmarshal(twd.Data, &stepData))
			assert.Equal(t, targets[i].ID, stepData.Foo)
		}
		// done monitoring out channels now
		outDone <-struct{}{}
		testingWg.Done()
	}()

	// pause when all were submitted
	inputWg.Wait()
	d.pause()

	// wait for pausing and all testing of pause result to be done
	testingWg.Wait()
	assert.NoError(t, goroutine_leak_check.CheckLeakedGoRoutines())
}
