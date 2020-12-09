package job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStateContext(t *testing.T) {
	ctx := newStateContext()
	require.NotNil(t, ctx)

	require.NoError(t, ctx.Err())
	require.Equal(t, StateActive, ctx.State())

	timeoutCtx, _ := context.WithTimeout(context.Background(), 10*time.Microsecond)

	var blockedOnDone bool
	select {
	case <-timeoutCtx.Done():
		blockedOnDone = true
	case <-ctx.Done():
	}
	require.True(t, blockedOnDone)
}

func TestStateContextActions(t *testing.T) {
	testScenario := func(action func(ctx StateContext), expectedState State, expectedError error) {
		ctx := newStateContext()
		require.NotNil(t, ctx)

		action(ctx)

		<-ctx.Done()

		require.Equal(t, expectedState, ctx.State())
		require.Equal(t, expectedError, ctx.Err())
	}

	t.Run("pause", func(t *testing.T) {
		testScenario(func(ctx StateContext) {
			ctx.pause()
		}, StatePaused, ErrPaused)
	})

	t.Run("cancel", func(t *testing.T) {
		testScenario(func(ctx StateContext) {
			ctx.cancel()
		}, StateCanceled, ErrCanceled)
	})
}
