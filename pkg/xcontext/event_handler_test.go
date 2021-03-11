package xcontext

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackgroundContext(t *testing.T) {
	ctx := Background()

	var blocked bool
	select {
	case <-ctx.WaitFor():
	default:
		blocked = true
	}

	require.True(t, blocked)
	require.Nil(t, ctx.Err())
	require.Nil(t, ctx.Notifications())
}

func TestContextCanceled(t *testing.T) {
	ctx, cancelFunc := WithCancel(nil)
	require.NotNil(t, ctx)

	cancelFunc()

	<-ctx.Done()
	<-ctx.WaitFor()

	require.Equal(t, Canceled, ctx.Err())
	require.Nil(t, ctx.Notifications())

	var paused bool
	select {
	case <-ctx.WaitFor(Paused):
		paused = true
	default:
	}
	require.False(t, paused)
}

func TestContextPaused(t *testing.T) {
	ctx, pauseFunc := WithNotify(nil, Paused)
	require.NotNil(t, ctx)

	pauseFunc()

	<-ctx.WaitFor(Paused)
	<-ctx.WaitFor()

	require.Nil(t, ctx.Err())
	require.Equal(t, []error{Paused}, ctx.Notifications())

	var canceled bool
	select {
	case <-ctx.Done():
		canceled = true
	default:
	}
	require.False(t, canceled)
}

func TestMultipleCancelPause(t *testing.T) {
	ctx, cancelFunc := WithCancel(nil)
	ctx, pauseFunc := WithNotify(ctx, Paused)
	require.NotNil(t, ctx)

	pauseFunc()
	cancelFunc()
	pauseFunc()
	cancelFunc()

	<-ctx.Done()
	<-ctx.WaitFor()
	<-ctx.WaitFor(Canceled, Paused)
	<-ctx.WaitFor(Paused)

	require.Equal(t, Canceled, ctx.Err())
	require.Equal(t, []error{Paused, Paused}, ctx.Notifications())
}

func TestGrandGrandGrandChild(t *testing.T) {
	ctx0, notifyFunc := WithNotify(nil, Paused)
	ctx1, _ := WithCancel(ctx0.Clone())
	ctx2, _ := WithNotify(ctx1.Clone(), Paused)

	require.False(t, ctx2.IsSignaledWith(Paused))
	notifyFunc()
	<-ctx2.WaitFor(Paused)
}

func TestWithParent(t *testing.T) {
	t.Run("pause_propagated", func(t *testing.T) {
		parentCtx, pauseFunc := WithNotify(nil, Paused)
		childCtx := parentCtx.Clone()

		pauseFunc()
		childCtx.WaitFor(Paused)
		require.Equal(t, []error{Paused}, childCtx.Notifications())
		<-childCtx.WaitFor()

		require.Nil(t, childCtx.Err())
	})
	t.Run("cancel_propagated", func(t *testing.T) {
		parentCtx, parentCancelFunc := WithCancel(nil)
		childCtx := parentCtx.Clone()

		parentCancelFunc()
		<-childCtx.Done()
		require.Equal(t, Canceled, childCtx.Err())
		<-childCtx.WaitFor()

		require.Nil(t, childCtx.Notifications())
	})
	t.Run("pause_cancel_happened_before_context_created", func(t *testing.T) {
		parentCtx, cancelFunc := WithCancel(nil)
		parentCtx, pauseFunc := WithNotify(parentCtx, Paused)
		pauseFunc()
		cancelFunc()

		childCtx := parentCtx.Clone()
		<-childCtx.Done()
		<-childCtx.WaitFor(Paused)
		<-childCtx.WaitFor()

		require.Equal(t, Canceled, childCtx.Err())
		require.Equal(t, []error{Paused}, childCtx.Notifications())
	})
	t.Run("child_pause_cancel_do_not_affect_parent", func(t *testing.T) {
		parentCtxs := map[string]Context{}
		parentCtxs["background"] = Background()
		parentCtxs["with_cancel"], _ = WithCancel(nil)
		for parentName, parentCtx := range parentCtxs {
			t.Run(parentName, func(t *testing.T) {
				childCtx, cancelFunc := WithCancel(parentCtx.Clone())
				childCtx, pauseFunc := WithNotify(childCtx, Paused)

				pauseFunc()
				cancelFunc()

				var blocked bool
				select {
				case <-parentCtx.WaitFor():
				default:
					blocked = true
				}

				require.True(t, blocked)
			})
		}
	})
}
