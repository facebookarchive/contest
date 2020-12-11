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
	case <-ctx.PausedOrDone():
	case <-ctx.Paused():
	}
	require.True(t, blockedOnDone)

	require.Nil(t, ctx.Err())
	require.Nil(t, ctx.PausedCtx().Err())
	require.Nil(t, ctx.PausedOrDoneCtx().Err())
}

func TestContextCanceled(t *testing.T) {
	ctx, _, cancel := NewContext()
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
	ctx, pause, _ := NewContext()
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
	ctx, pause, cancel := NewContext()
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
