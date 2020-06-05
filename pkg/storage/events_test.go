// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"os"
	"testing"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/stretchr/testify/require"
)

var (
	header = testevent.Header{
		JobID: types.JobID(123),
		RunID: types.RunID(456),
		TestName: "TestStep",
		StepLabel: "TestLabel",
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
)

type nullStorage struct{}

func (n *nullStorage) StoreJobRequest(request *job.Request) (types.JobID, error) {
	return types.JobID(0), nil
}
func (n *nullStorage) GetJobRequest(jobID types.JobID) (*job.Request, error)  { return nil, nil }
func (n *nullStorage) StoreJobReport(report *job.JobReport) error             { return nil }
func (n *nullStorage) GetJobReport(jobID types.JobID) (*job.JobReport, error) { return nil, nil }
func (n *nullStorage) StoreTestEvent(event testevent.Event) error             { return nil }
func (n *nullStorage) GetTestEvents(eventQuery *testevent.Query) ([]testevent.Event, error) {
	return nil, nil
}
func (n *nullStorage) StoreFrameworkEvent(event frameworkevent.Event) error { return nil }
func (n *nullStorage) GetFrameworkEvent(eventQuery *frameworkevent.Query) ([]frameworkevent.Event, error) {
	return nil, nil
}

func (m *nullStorage) Version() (int64, error) {
	return 0, nil
}

func TestMain(m *testing.M) {
	SetStorage(&nullStorage{})
	os.Exit(m.Run())
}

func TestEmitUnrestricted(t *testing.T) {
	em := NewTestEventEmitter(header)
	require.NoError(t, em.Emit(testevent.Data{EventName: allowedEvents[0]}))
	require.NoError(t, em.Emit(testevent.Data{EventName: allowedEvents[1]}))
	require.NoError(t, em.Emit(testevent.Data{EventName: forbiddenEvents[0]}))
}

func TestEmitRestricted(t *testing.T) {
	em := NewTestEventEmitterWithAllowedEvents(header, &allowedMap)
	require.NoError(t, em.Emit(testevent.Data{EventName: allowedEvents[0]}))
	require.NoError(t, em.Emit(testevent.Data{EventName: allowedEvents[1]}))
	require.Error(t, em.Emit(testevent.Data{EventName: forbiddenEvents[0]}))
}
