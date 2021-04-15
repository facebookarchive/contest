// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"fmt"
	"os"
	"testing"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

var (
	header = testevent.Header{
		JobID:         types.JobID(123),
		RunID:         types.RunID(456),
		TestName:      "TestStep",
		TestStepLabel: "TestLabel",
	}
	allowedEvents = []event.Name{
		"TestEventAllowed1",
		"TestEventAllowed2",
	}
	allowedMap = map[event.Name]bool{
		allowedEvents[0]: true,
		allowedEvents[1]: true,
	}
	forbiddenEvents = []event.Name{
		"TestEventForbidden1",
	}
	ctx = logrusctx.NewContext(logger.LevelDebug)
)

type nullStorage struct{}

func (n *nullStorage) StoreJobRequest(ctx xcontext.Context, request *job.Request) (types.JobID, error) {
	return types.JobID(0), nil
}
func (n *nullStorage) GetJobRequest(ctx xcontext.Context, jobID types.JobID) (*job.Request, error) {
	return nil, nil
}
func (n *nullStorage) StoreReport(ctx xcontext.Context, report *job.Report) error { return nil }
func (n *nullStorage) GetJobReport(ctx xcontext.Context, jobID types.JobID) (*job.JobReport, error) {
	return nil, nil
}
func (n *nullStorage) ListJobs(ctx xcontext.Context, query *JobQuery) ([]types.JobID, error) {
	return nil, nil
}
func (n *nullStorage) StoreTestEvent(ctx xcontext.Context, event testevent.Event) error { return nil }
func (n *nullStorage) GetTestEvents(ctx xcontext.Context, eventQuery *testevent.Query) ([]testevent.Event, error) {
	return nil, nil
}
func (n *nullStorage) StoreFrameworkEvent(ctx xcontext.Context, event frameworkevent.Event) error {
	return nil
}
func (n *nullStorage) GetFrameworkEvent(ctx xcontext.Context, eventQuery *frameworkevent.Query) ([]frameworkevent.Event, error) {
	return nil, nil
}

func (n *nullStorage) Version() (uint64, error) {
	return 0, nil
}

func TestMain(m *testing.M) {
	if err := SetStorage(&nullStorage{}); err != nil {
		panic(fmt.Sprintf("could not configure storage: %v", err))
	}
	os.Exit(m.Run())
}

func TestEmitUnrestricted(t *testing.T) {
	em := NewTestEventEmitter(header)
	require.NoError(t, em.Emit(ctx, testevent.Data{EventName: allowedEvents[0]}))
	require.NoError(t, em.Emit(ctx, testevent.Data{EventName: allowedEvents[1]}))
	require.NoError(t, em.Emit(ctx, testevent.Data{EventName: forbiddenEvents[0]}))
}

func TestEmitRestricted(t *testing.T) {
	em := NewTestEventEmitterWithAllowedEvents(header, &allowedMap)
	require.NoError(t, em.Emit(ctx, testevent.Data{EventName: allowedEvents[0]}))
	require.NoError(t, em.Emit(ctx, testevent.Data{EventName: allowedEvents[1]}))
	require.Error(t, em.Emit(ctx, testevent.Data{EventName: forbiddenEvents[0]}))
}
