// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	reporterNoop "github.com/facebookincubator/contest/plugins/reporters/noop"
	targetLockerNoop "github.com/facebookincubator/contest/plugins/targetlocker/noop"
	"golang.org/x/xerrors"

	"github.com/stretchr/testify/require"
)

// Definition of two dummy TestSteps to be used to test the PluginRegistry

// AStep implements a dummy TestStep
type AStep struct{}

// NewAStep initializes a new AStep
func NewAStep() test.TestStep {
	return &AStep{}
}

// ValidateParameters validates the parameters for the AStep
func (e AStep) ValidateParameters(params test.TestStepParameters) error {
	return nil
}

// Name returns the name of the AStep
func (e AStep) Name() string {
	return "AStep"
}

// Run executes the AStep
func (e AStep) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	return nil
}

// CanResume tells whether this step is able to resume.
func (e AStep) CanResume() bool {
	return false
}

// Resume tries to resume AStep
func (e AStep) Resume(cancel, pause <-chan struct{}, _ test.TestStepChannels, _ test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: "AStep"}
}

type testStepFactoryGood struct{}

func (f *testStepFactoryGood) New() test.TestStep               { return nil }
func (f *testStepFactoryGood) Events() []event.Name             { return []event.Name{"validEventName"} }
func (f *testStepFactoryGood) UniqueImplementationName() string { return "unit-test-positive" }

type testStepFactoryBadEventName struct{}

func (f *testStepFactoryBadEventName) New() test.TestStep   { return nil }
func (f *testStepFactoryBadEventName) Events() []event.Name { return []event.Name{"invalid event name"} }
func (f *testStepFactoryBadEventName) UniqueImplementationName() string {
	return "unit-test-neg-invalid-event-name"
}

func TestRegisterTestStepEvents(t *testing.T) {
	pr := NewPluginRegistry()
	t.Run("positive", func(t *testing.T) {
		err := pr.RegisterFactory(&testStepFactoryGood{})
		require.NoError(t, err)
	})
	t.Run("negative", func(t *testing.T) {
		t.Run("invalid_event_name", func(t *testing.T) {
			err := pr.RegisterFactory(&testStepFactoryBadEventName{})
			require.Error(t, err)
			require.True(t, xerrors.As(err, &event.ErrInvalidEventName{}))
		})
	})
}

func TestPluginRegistry_RegisterFactoriesAsType(t *testing.T) {
	pr := NewPluginRegistry()
	t.Run("positive", func(t *testing.T) {
		err := pr.RegisterFactoriesAsType((*target.LockerFactory)(nil), target.LockerFactories{&targetLockerNoop.Factory{}}.ToAbstract())
		require.NoError(t, err)
		targetLocker, err := pr.NewTargetLocker("noop", time.Second, "")
		require.NoError(t, err)
		require.NotNil(t, targetLocker)
	})
	t.Run("negative", func(t *testing.T) {
		t.Run("WrongFactoryType", func(t *testing.T) {
			err := pr.RegisterFactoriesAsType((*target.LockerFactory)(nil), job.ReporterFactories{&reporterNoop.Factory{}}.ToAbstract())
			require.True(t, xerrors.As(err, &ErrInvalidFactoryType{}))
		})
		t.Run("InvalidFactoryType", func(t *testing.T) {
			err := pr.RegisterFactoriesAsType((target.LockerFactory)(nil), target.LockerFactories{&targetLockerNoop.Factory{}}.ToAbstract())
			require.True(t, xerrors.As(err, &ErrInvalidFactoryType{}))
		})
		t.Run("NilFactory", func(t *testing.T) {
			err := pr.RegisterFactoriesAsType((*target.LockerFactory)(nil), target.LockerFactories{nil}.ToAbstract())
			require.True(t, xerrors.As(err, &ErrNilFactory{}))
		})
	})
}

type fakeFactory struct{}

func (f *fakeFactory) UniqueImplementationName() string { return "unit-test" }

func TestNewPluginRegistry_RegisterFactory(t *testing.T) {
	pr := NewPluginRegistry()
	t.Run("positive", func(t *testing.T) {
		err := pr.RegisterFactories(target.LockerFactories{&targetLockerNoop.Factory{}}.ToAbstract())
		require.NoError(t, err)
		targetLocker, err := pr.NewTargetLocker("noop", time.Second, "")
		require.NoError(t, err)
		require.NotNil(t, targetLocker)
	})
	t.Run("negative", func(t *testing.T) {
		t.Run("UnknownFactoryType", func(t *testing.T) {
			err := pr.RegisterFactory(&fakeFactory{})
			require.True(t, xerrors.As(err, &ErrUnknownFactoryType{}))
		})
	})
}
