package statectx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCancellation(t *testing.T) {
	ctx, cancel := newCancelContext(context.Background())
	require.NotNil(t, ctx)
	require.NotNil(t, cancel)

	var blocked bool
	select {
	case <-ctx.Done():
		blocked = true
	default:
	}
	require.False(t, blocked)

	cancel(ErrCanceled)
	<-ctx.Done()
	require.Equal(t, ErrCanceled, ctx.Err())

	// further cancels do not affect the error code
	cancel(ErrPaused)
	require.Equal(t, ErrCanceled, ctx.Err())
}

func TestAbstractParentContext(t *testing.T) {
	parCtx, parCancel := context.WithCancel(context.Background())
	ctx, cancel := newCancelContext(parCtx)

	parCancel()
	<-ctx.Done()
	require.Equal(t, context.Canceled, ctx.Err())

	cancel(ErrPaused)
	require.Equal(t, context.Canceled, ctx.Err())
}

func TestCancelContextAsParentContext(t *testing.T) {
	t.Run("parent_cancel_propagated", func(t *testing.T) {
		parCtx, parCancel := newCancelContext(context.Background())
		ctx, cancel := newCancelContext(parCtx)

		parCancel(ErrCanceled)
		<-ctx.Done()
		require.Equal(t, ErrCanceled, ctx.Err())

		parCancel(ErrPaused)
		cancel(ErrPaused)
		require.Equal(t, ErrCanceled, ctx.Err())
	})

	t.Run("child_cancel_does_not_affect_parent", func(t *testing.T) {
		parCtx, parCancel := newCancelContext(context.Background())
		ctx, cancel := newCancelContext(parCtx)

		cancel(ErrCanceled)
		<-ctx.Done()
		require.Equal(t, ErrCanceled, ctx.Err())

		var blocked bool
		select {
		case <-parCtx.Done():
			blocked = true
		default:
		}
		require.False(t, blocked)
		require.Nil(t, parCtx.Err())

		parCancel(ErrPaused)
		<-parCtx.Done()
		require.Equal(t, ErrPaused, parCtx.Err())
		require.Equal(t, ErrCanceled, ctx.Err())
	})
}

func TestParentInitiallyCancelled(t *testing.T) {
	t.Run("unknown_parent_context", func(t *testing.T) {
		parCtx, parCancel := context.WithCancel(context.Background())
		parCancel()

		ctx, cancel := newCancelContext(parCtx)
		require.NotNil(t, ctx)
		require.NotNil(t, cancel)

		<-ctx.Done()
		require.Equal(t, context.Canceled, ctx.Err())

		cancel(ErrPaused) // should be no effect
		require.Equal(t, context.Canceled, ctx.Err())
	})

	t.Run("cancel_parent_context", func(t *testing.T) {
		parCtx, parCancel := newCancelContext(context.Background())
		parCancel(ErrCanceled)

		ctx, cancel := newCancelContext(parCtx)
		require.NotNil(t, ctx)
		require.NotNil(t, cancel)

		<-ctx.Done()
		require.Equal(t, ErrCanceled, ctx.Err())

		cancel(ErrPaused) // should be no effect
		require.Equal(t, ErrCanceled, ctx.Err())
	})
}
