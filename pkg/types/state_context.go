// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package types

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCanceled is returned by Context.Err when the context was canceled
var ErrCanceled = errors.New("job is canceled")

// ErrPaused is returned by Context.Err when the context was paused
var ErrPaused = errors.New("job is paused")

type State int

var StateActive State = 0
var StatePaused State = 1
var StateCanceled State = 2

type StateContext interface {
	context.Context

	State() State

	pause()
	cancel()
}

func NewStateContext() (ctx StateContext, pause func(), cancel func()) {
	resCtx := &stateCtx{
		done:  make(chan struct{}),
		state: StateActive,
	}
	return resCtx, func() { resCtx.pause() }, func() { resCtx.cancel() }
}

type stateCtx struct {
	done chan struct{}

	mu    sync.Mutex
	state State
}

func (*stateCtx) Deadline() (deadline time.Time, ok bool) {
	return
}

func (c *stateCtx) Value( /*key*/ interface{}) interface{} {
	return nil
}

func (c *stateCtx) Done() <-chan struct{} {
	return c.done
}

func (c *stateCtx) Err() error {
	state := c.State()
	switch state {
	case StateActive:
		return nil
	case StatePaused:
		return ErrPaused
	case StateCanceled:
		return ErrCanceled
	}

	panic(fmt.Sprintf("stateCtx: internal error: unknown state: %d", state))
}

func (c *stateCtx) State() State {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()
	return state
}

func (c *stateCtx) setFinalState(state State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != StateActive {
		return // already canceled or paused
	}
	c.state = state
	close(c.done)
}

// cancel sets c.state to Canceled, closes c.done
func (c *stateCtx) cancel() {
	c.setFinalState(StateCanceled)
}

func (c *stateCtx) pause() {
	c.setFinalState(StatePaused)
}
