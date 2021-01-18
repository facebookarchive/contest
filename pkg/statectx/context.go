// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package statectx

import (
	"context"
	"errors"
	"time"
)

// ErrCanceled is returned by Context.Err when the context was canceled
var ErrCanceled = errors.New("job is canceled")

// ErrPaused is returned by Context.Err when the context was paused
var ErrPaused = errors.New("job is paused")

// Context implements context.Context interface that acts as context for cancellation signal
// It also implements:
// - Paused()/PausedCtx() that alert in case of pause happened
// - PausedOrDone()/PausedOrDoneCtx() that alert in case of pause/cancel events
type Context interface {
	context.Context

	Paused() <-chan struct{}
	PausedCtx() context.Context

	PausedOrDone() <-chan struct{}
	PausedOrDoneCtx() context.Context
}

func Background() Context {
	return &stateCtx{
		cancelCtx:      context.Background(),
		pauseCtx:       context.Background(),
		pauseOrDoneCtx: context.Background(),
	}
}

func newInternal(cctx, pctx, cpctx context.Context) (Context, func(), func()) {
	cancelCtx, cancel := newCancelContext(cctx)
	pauseCtx, pause := newCancelContext(pctx)
	pauseOrDoneCtx, pauseOrDone := newCancelContext(cpctx)

	resCtx := &stateCtx{
		cancelCtx:      cancelCtx,
		pauseCtx:       pauseCtx,
		pauseOrDoneCtx: pauseOrDoneCtx,
	}

	wrap := func(action func(err error), err error) func() {
		return func() {
			pauseOrDone(err)
			action(err)
		}
	}
	return resCtx, wrap(pause, ErrPaused), wrap(cancel, ErrCanceled)
}

func New() (Context, func(), func()) {
	return newInternal(context.Background(), context.Background(), context.Background())
}

func WithParent(ctx Context) (Context, func(), func()) {
	if ctx == nil {
		return New()
	}
	return newInternal(ctx, ctx.PausedCtx(), ctx.PausedOrDoneCtx())
}

type stateCtx struct {
	cancelCtx      context.Context
	pauseCtx       context.Context
	pauseOrDoneCtx context.Context
}

func (c *stateCtx) Deadline() (deadline time.Time, ok bool) {
	return c.cancelCtx.Deadline()
}

func (c *stateCtx) Value(key interface{}) interface{} {
	return c.cancelCtx.Value(key)
}

func (c *stateCtx) Done() <-chan struct{} {
	return c.cancelCtx.Done()
}

func (c *stateCtx) Err() error {
	return c.cancelCtx.Err()
}

func (c *stateCtx) Paused() <-chan struct{} {
	return c.pauseCtx.Done()
}

func (c *stateCtx) PausedCtx() context.Context {
	return c.pauseCtx
}

func (c *stateCtx) PausedOrDone() <-chan struct{} {
	return c.pauseOrDoneCtx.Done()
}

func (c *stateCtx) PausedOrDoneCtx() context.Context {
	return c.pauseOrDoneCtx
}
