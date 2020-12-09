package types

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStateContext(t *testing.T) {
	ctx, _, _ := NewStateContext()
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
	testScenario := func(action func(pause func(), cancel func()), expectedState State, expectedError error) {
		ctx, pause, cancel := NewStateContext()
		require.NotNil(t, ctx)

		action(pause, cancel)

		<-ctx.Done()

		require.Equal(t, expectedState, ctx.State())
		require.Equal(t, expectedError, ctx.Err())
	}

	t.Run("pause", func(t *testing.T) {
		testScenario(func(pause func(), cancel func()) {
			pause()
		}, StatePaused, ErrPaused)
	})

	t.Run("cancel", func(t *testing.T) {
		testScenario(func(pause func(), cancel func()) {
			cancel()
		}, StateCanceled, ErrCanceled)
	})
}
