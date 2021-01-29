package statectx

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/contest/tests/common"
)

func TestBackgroundContext(t *testing.T) {
	ctx := Background()

	var blocked bool
	select {
	case <-time.After(50 * time.Millisecond):
		blocked = true
	case <-ctx.Done():
	case <-ctx.PausedOrDone():
	case <-ctx.Paused():
	}

	require.True(t, blocked)
	require.Nil(t, ctx.Err())
	require.Nil(t, ctx.PausedCtx().Err())
	require.Nil(t, ctx.PausedOrDoneCtx().Err())
}

func TestContextBlocked(t *testing.T) {
	ctx, _, _ := New()
	require.NotNil(t, ctx)

	require.NoError(t, ctx.Err())

	var blockedOnDone bool
	select {
	case <-time.After(50 * time.Millisecond):
		blockedOnDone = true
	case <-ctx.Done():
	case <-ctx.PausedOrDone():
	case <-ctx.Paused():
	}
	require.True(t, blockedOnDone)

	require.Nil(t, ctx.Err())
	require.Nil(t, ctx.PausedCtx().Err())
	require.Nil(t, ctx.PausedOrDoneCtx().Err())
}

func TestContextCanceled(t *testing.T) {
	ctx, _, cancel := New()
	require.NotNil(t, ctx)

	cancel()

	<-ctx.Done()
	<-ctx.PausedOrDone()

	require.Equal(t, ErrCanceled, ctx.Err())
	require.Nil(t, ctx.PausedCtx().Err())
	require.Equal(t, ErrCanceled, ctx.PausedOrDoneCtx().Err())

	var paused bool
	select {
	case <-ctx.Paused():
		paused = true
	default:
	}
	require.False(t, paused)
}

func TestContextPaused(t *testing.T) {
	ctx, pause, _ := New()
	require.NotNil(t, ctx)

	pause()

	<-ctx.Paused()
	<-ctx.PausedOrDone()

	require.Nil(t, ctx.Err())
	require.Equal(t, ErrPaused, ctx.PausedCtx().Err())
	require.Equal(t, ErrPaused, ctx.PausedOrDoneCtx().Err())

	var canceled bool
	select {
	case <-ctx.Done():
		canceled = true
	default:
	}
	require.False(t, canceled)
}

func TestMultipleCancelPause(t *testing.T) {
	ctx, pause, cancel := New()
	require.NotNil(t, ctx)

	pause()
	cancel()
	pause()
	cancel()

	<-ctx.Done()
	<-ctx.PausedOrDone()
	<-ctx.Paused()

	require.Equal(t, ErrCanceled, ctx.Err())
	require.Equal(t, ErrPaused, ctx.PausedCtx().Err())
	require.Equal(t, ErrPaused, ctx.PausedOrDoneCtx().Err())
}

func TestWithParent(t *testing.T) {
	t.Run("pause_propagated", func(t *testing.T) {
		parentCtx, parentPause, _ := New()
		childCtx, _, _ := WithParent(parentCtx)

		parentPause()
		<-childCtx.Paused()
		require.Equal(t, ErrPaused, childCtx.PausedCtx().Err())
		<-childCtx.PausedOrDone()
		require.Equal(t, ErrPaused, childCtx.PausedOrDoneCtx().Err())

		require.Nil(t, childCtx.Err())
	})
	t.Run("cancel_propagated", func(t *testing.T) {
		parentCtx, _, parentCancel := New()
		childCtx, _, _ := WithParent(parentCtx)

		parentCancel()
		<-childCtx.Done()
		require.Equal(t, ErrCanceled, childCtx.Err())
		<-childCtx.PausedOrDone()
		require.Equal(t, ErrCanceled, childCtx.PausedOrDoneCtx().Err())

		require.Nil(t, childCtx.PausedCtx().Err())
	})
	t.Run("pause_cancel_happened_before_context_created", func(t *testing.T) {
		parentCtx, parentPause, parentCancel := New()
		parentPause()
		parentCancel()

		childCtx, _, _ := WithParent(parentCtx)
		<-childCtx.Done()
		<-childCtx.Paused()
		<-childCtx.PausedOrDone()

		require.Equal(t, ErrCanceled, childCtx.Err())
		require.Equal(t, ErrPaused, childCtx.PausedCtx().Err())
		require.Equal(t, ErrPaused, childCtx.PausedOrDoneCtx().Err())
	})
	t.Run("child_pause_cancel_do_not_affect_parent", func(t *testing.T) {
		parentCtx, _, _ := New()
		_, childPause, childCancel := WithParent(parentCtx)

		childPause()
		childCancel()

		var blocked bool
		select {
		case <-parentCtx.Done():
		case <-parentCtx.Paused():
		case <-parentCtx.PausedOrDone():
		default:
			blocked = true
		}

		require.True(t, blocked)
	})
}

func TestMain(m *testing.M) {
	flag.Parse()
	common.LeakCheckingTestMain(m)
}
