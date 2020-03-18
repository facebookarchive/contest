// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package inmemory

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestInMemoryNew(t *testing.T) {
	tl, err := New(time.Second, "")
	require.NoError(t, err)
	require.NotNil(t, tl)
	require.IsType(t, &InMemory{}, tl)
}

func TestInMemoryLock(t *testing.T) {
	tl, err := New(time.Second, "")
	require.NoError(t, err)
	// we don't enforce that at least one target is passed, as checking on
	// non-zero targets is the framework's responsibility, not the plugin.
	// So, zero targets is OK.
	jobID := types.JobID(123)
	require.Nil(t, tl.Lock(jobID, nil))
	require.Nil(t, tl.Lock(jobID, []*target.Target{}))
	require.Nil(t, tl.Lock(jobID, []*target.Target{
		&target.Target{Name: "t1"},
	}))
	require.Nil(t, tl.Lock(jobID, []*target.Target{
		&target.Target{Name: "t2"},
		&target.Target{Name: "t3"},
	}))
}

func TestInMemoryUnlock(t *testing.T) {
	tl, err := New(time.Second, "")
	require.NoError(t, err)
	// we don't enforce that at least one target is passed, as checking on
	// non-zero targets is the framework's responsibility, not the plugin.
	// So, zero targets is OK.
	jobID := types.JobID(123)
	require.Nil(t, tl.Unlock(jobID, nil))
	require.Nil(t, tl.Unlock(jobID, []*target.Target{}))
	require.Nil(t, tl.Unlock(jobID, []*target.Target{
		&target.Target{Name: "blah"},
	}))
	require.Nil(t, tl.Unlock(jobID, []*target.Target{
		&target.Target{Name: "blah"},
		&target.Target{Name: "bleh"},
	}))
}
