// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func tryLeak() {
	ctx := Background()
	ctx, _ = WithCancel(ctx)
	ctx, _ = WithNotify(ctx, ErrPaused)
	ctx.Until(nil)
	ctx = WithResetSignalers(ctx)
	ctx = WithStdContext(ctx, context.Background())
	ctx.StdCtxUntil(nil)
	_ = ctx
}

func gc() {
	// GC in Go is very tricky, and to be sure everything
	// is cleaned up, we call it few times.
	for i := 0; i < 3; i++ {
		runtime.GC()
		runtime.Gosched()
	}
}

func TestGoroutineLeak(t *testing.T) {
	gc()
	old := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		tryLeak()
		gc()
	}

	stack := make([]byte, 65536)
	n := runtime.Stack(stack, true)
	stack = stack[:n]
	require.GreaterOrEqual(t, old, runtime.NumGoroutine(), fmt.Sprintf("%s", stack))
}

func TestStdCtxUntil(t *testing.T) {
	ctx, pauseFn := WithNotify(nil, ErrPaused)
	stdCtx := ctx.StdCtxUntil(nil)

	select {
	case <-stdCtx.Done():
		t.Fatal("stdCtx should not be Done by now")
	default:
	}

	pauseFn()
	<-stdCtx.Done()
}
