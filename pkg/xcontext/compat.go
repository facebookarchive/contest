// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// This is a temporary file, to make migration to xcontext (from statectx)
// smoother.

package xcontext

import (
	"context"
)

var ErrCanceled = Canceled
var ErrPaused = Paused

func (ctx *ctxValue) Paused() <-chan struct{} {
	return ctx.Until(Paused)
}

func (ctx *ctxValue) PausedOrDone() <-chan struct{} {
	return ctx.Until(nil)
}

func (ctx *ctxValue) PausedCtx() context.Context {
	return ctx.StdCtxUntil(Paused)
}

func New() (Context, context.CancelFunc, context.CancelFunc) {
	return WithParent(nil)
}

func WithParent(parent Context) (Context, context.CancelFunc, context.CancelFunc) {
	ctx, cancelFunc := WithCancel(parent)
	ctx, pauseFunc := WithNotify(ctx, Paused)
	return ctx, pauseFunc, cancelFunc
}
