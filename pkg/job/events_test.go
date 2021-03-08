// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

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
