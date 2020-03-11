// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/abstract"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	reportersNoop "github.com/facebookincubator/contest/plugins/reporters/noop"
	targetLockerNoop "github.com/facebookincubator/contest/plugins/targetlocker/noop"
	"github.com/facebookincubator/contest/plugins/targetmanagers/targetlist"
	"github.com/facebookincubator/contest/plugins/testfetchers/uri"
	"github.com/facebookincubator/contest/plugins/teststeps/example"
	"golang.org/x/xerrors"

	"github.com/stretchr/testify/require"
)

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

type fakeFactory struct{}

func (f *fakeFactory) UniqueImplementationName() string { return "unit-test" }

func TestNewPluginRegistry_RegisterFactory(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		pr := NewPluginRegistry()
		err := pr.RegisterFactories(target.LockerFactories{&targetLockerNoop.Factory{}}.ToAbstract())
		require.NoError(t, err)
		targetLocker, err := pr.NewTargetLocker("noop", time.Second, "")
		require.NoError(t, err)
		require.NotNil(t, targetLocker)
	})

	// Check if all factory types are registered and then are used/called correctly.
	t.Run("all_types", func(t *testing.T) {
		// See also main.TestValidateFactories

		pr := NewPluginRegistry()
		factoryMap := map[abstract.Factory]func(pluginName string) (interface{}, error){
			&targetLockerNoop.Factory{}: func(pluginName string) (interface{}, error) {
				return pr.NewTargetLocker(pluginName, 0, "")
			},
			&targetlist.Factory{}: func(pluginName string) (interface{}, error) {
				return pr.NewTargetManager(pluginName)
			},
			&reportersNoop.Factory{}: func(pluginName string) (interface{}, error) {
				return pr.NewReporter(pluginName)
			},
			&example.Factory{}: func(pluginName string) (interface{}, error) {
				r, _, err := pr.NewTestStep(pluginName)
				return r, err
			},
			&uri.Factory{}: func(pluginName string) (interface{}, error) {
				return pr.NewTestFetcher(pluginName)
			},
		}

		// Register factories and require for all existing factory types to be presented in this test.
		require.Equal(t, MaxFactoryType, len(factoryMap))
		usedFactoryTypes := map[FactoryType]struct{}{}
		for factory := range factoryMap {
			err := pr.RegisterFactory(factory)
			require.NoError(t, err)
			usedFactoryTypes[factoryTypeOf(factory)] = struct{}{}
		}
		require.Equal(t, MaxFactoryType, len(usedFactoryTypes))

		// Call the factories and verify the result.
		for factory, checkFunc := range factoryMap {
			returnedValue, err := checkFunc(factory.UniqueImplementationName())
			require.NoError(t, err)
			require.NotNil(t, returnedValue)
		}
	})

	t.Run("negative", func(t *testing.T) {
		pr := NewPluginRegistry()
		t.Run("UnknownFactoryType_unknownFactory", func(t *testing.T) {
			err := pr.RegisterFactory(&fakeFactory{})
			require.True(t, xerrors.As(err, &ErrUnknownFactoryType{}))
		})
		t.Run("UnknownFactoryType_nilFactory", func(t *testing.T) {
			err := pr.RegisterFactory(nil)
			require.Error(t, err)
			require.True(t, xerrors.As(err, &ErrUnknownFactoryType{}), err)
		})
	})
}
