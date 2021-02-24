package job

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/contest/pkg/event"
)

func TestEventToJobStateMapping(t *testing.T) {
	m := map[State]event.Name{}
	for _, e := range JobStateEvents {
		st, err := EventNameToJobState(e)
		require.NoError(t, err)
		m[st] = e
	}
	require.Equal(t, 8, len(m))
	st, err := EventNameToJobState(event.Name("foo"))
	require.Error(t, err)
	require.Equal(t, JobStateUnknown, st)
	for k, v := range m {
		require.Equal(t, k.String(), string(v))
	}
	require.Equal(t, JobStateUnknown.String(), "JobStateUnknown")
}
