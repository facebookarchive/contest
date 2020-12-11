package statectx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestContextBlocked(t *testing.T) {
	ctx, _, _ := NewContext()
	require.NotNil(t, ctx)

	require.NoError(t, ctx.Err())

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 10*time.Microsecond)
	defer timeoutCancel()

	var blockedOnDone bool
	select {
	case <-timeoutCtx.Done():
		blockedOnDone = true
	case <-ctx.Done():
	case <-ctx.Any():
	case <-ctx.OnPause():
	}
	require.True(t, blockedOnDone)

	require.Nil(t, ctx.Err())
	require.Nil(t, ctx.PauseCtx().Err())
	require.Nil(t, ctx.AnyCtx().Err())
}

func TestContextCanceled(t *testing.T) {
	ctx, _, cancel := NewContext()
	require.NotNil(t, ctx)

	cancel()

	<-ctx.Done()
	<-ctx.Any()

	require.Equal(t, ErrCanceled, ctx.Err())
	require.Nil(t, ctx.PauseCtx().Err())
	require.Equal(t, ErrCanceled, ctx.AnyCtx().Err())

	var paused bool
	select {
	case <-ctx.OnPause():
		paused = true
	default:
	}
	require.False(t, paused)
}

func TestContextPaused(t *testing.T) {
	ctx, pause, _ := NewContext()
	require.NotNil(t, ctx)

	pause()

	<-ctx.OnPause()
	<-ctx.Any()

	require.Nil(t, ctx.Err())
	require.Equal(t, ErrPaused, ctx.PauseCtx().Err())
	require.Equal(t, ErrPaused, ctx.AnyCtx().Err())

	var canceled bool
	select {
	case <-ctx.Done():
		canceled = true
	default:
	}
	require.False(t, canceled)
}
