// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

// emptyFrameworkEventManager does not emit or fetch anything
type emptyFrameworkEventManager struct{}

func (fem emptyFrameworkEventManager) Emit(ctx xcontext.Context, event frameworkevent.Event) error {
	return nil
}
func (fem emptyFrameworkEventManager) Fetch(ctx xcontext.Context, fields ...frameworkevent.QueryField) ([]frameworkevent.Event, error) {
	return nil, nil
}

func (fem emptyFrameworkEventManager) FetchAsync(ctx xcontext.Context, fields ...frameworkevent.QueryField) ([]frameworkevent.Event, error) {
	return nil, nil
}

func TestGetCurrentRunNoEvents(t *testing.T) {
	mockRunner := JobRunner{
		targetMap:             nil,
		targetLock:            &sync.RWMutex{},
		frameworkEventManager: emptyFrameworkEventManager{},
		testEvManager:         storage.TestEventFetcher{},
	}
	// request a job that does not have any events at all
	runID, err := mockRunner.GetCurrentRun(xcontext.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, types.RunID(0), runID)
}
