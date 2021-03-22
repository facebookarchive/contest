// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCancellation(t *testing.T) {
	ctx, cancelFunc := WithCancel(Background())
	require.NotNil(t, ctx)

	ctx, customCancelFunc := WithCancel(ctx, errors.New("custom-signal"))

	var blocked bool
	select {
	case <-ctx.Done():
		blocked = true
	default:
	}
	require.False(t, blocked)

	cancelFunc()
	<-ctx.Done()
	require.Equal(t, Canceled, ctx.Err())

	// further cancels do not affect the error code
	customCancelFunc()
	require.Equal(t, Canceled, ctx.Err())
}

func TestAbstractParentContext(t *testing.T) {
	parCtx, parCancel := context.WithCancel(context.Background())
	ctx := NewContext(parCtx, "", nil, nil, nil, nil, nil)
	child, cancelFunc := WithCancel(ctx, errors.New("custom-signal"))

	parCancel()
	<-ctx.Done()
	<-child.Done()
	require.Equal(t, context.Canceled, child.Err())

	cancelFunc()
	require.Equal(t, context.Canceled, child.Err())
}

func TestCancelContextAsParentContext(t *testing.T) {
	t.Run("parent_cancel_propagated", func(t *testing.T) {
		parCtx, parCancelFunc0 := WithCancel(Background())
		parCtx, parCancelFunc1 := WithCancel(parCtx, errors.New("custom-signal"))
		ctx, ctxCancelFunc := WithCancel(parCtx.Clone(), errors.New("custom-signal"))

		parCancelFunc0()
		<-ctx.Done()
		require.Equal(t, Canceled, ctx.Err())

		parCancelFunc1()
		ctxCancelFunc()
		require.Equal(t, Canceled, ctx.Err())
	})

	t.Run("child_cancel_does_not_affect_parent", func(t *testing.T) {
		customSignal := errors.New("custom-signal")

		parCtx, parCancelFunc := WithCancel(Background(), customSignal)
		ctx, ctxCancelFunc := WithCancel(parCtx.Clone())

		ctxCancelFunc()
		<-ctx.Done()
		require.Equal(t, Canceled, ctx.Err())

		var blocked bool
		select {
		case <-parCtx.Done():
		default:
			blocked = true
		}
		require.True(t, blocked)
		require.Nil(t, parCtx.Err())

		parCancelFunc()
		<-parCtx.Done()
		require.Equal(t, customSignal, parCtx.Err())
		require.Equal(t, Canceled, ctx.Err())
	})
}

func TestParentInitiallyCancelled(t *testing.T) {
	t.Run("unknown_parent_context", func(t *testing.T) {
		customSignal := errors.New("custom-signal")

		parCtx, parCancel := context.WithCancel(context.Background())
		parCancel()

		ctx, cancelFunc := WithCancel(Extend(parCtx), customSignal)
		require.NotNil(t, ctx)

		<-ctx.Done()
		require.Equal(t, context.Canceled, ctx.Err())

		cancelFunc() // should be no effect
		require.Equal(t, context.Canceled, ctx.Err())
	})

	t.Run("cancel_grand_parent_context", func(t *testing.T) {
		customSignal := errors.New("custom-signal")

		parCtx, parCancelFunc := WithCancel(nil)
		parCancelFunc()

		ctx, cancelFunc := WithCancel(parCtx.Clone(), customSignal)
		require.NotNil(t, ctx)

		<-ctx.Done()
		require.Equal(t, Canceled, ctx.Err())

		cancelFunc() // should be no effect
		require.Equal(t, Canceled, ctx.Err())
	})
}
