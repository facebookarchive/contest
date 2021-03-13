// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration

package tests

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/runner"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/teststeps/cmd"
	"github.com/facebookincubator/contest/plugins/teststeps/echo"
	"github.com/facebookincubator/contest/plugins/teststeps/example"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/channels"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/crash"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/fail"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/hanging"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/noreturn"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/panicstep"
)

var (
	pluginRegistry *pluginregistry.PluginRegistry
	targets        []*target.Target
	successTimeout = 5 * time.Second
)

var testSteps = map[string]test.TestStepFactory{
	echo.Name:      echo.New,
	example.Name:   example.New,
	panicstep.Name: panicstep.New,
	noreturn.Name:  noreturn.New,
	hanging.Name:   hanging.New,
	channels.Name:  channels.New,
	cmd.Name:       cmd.New,
	crash.Name:     crash.New,
	fail.Name:      fail.New,
}

var testStepsEvents = map[string][]event.Name{
	echo.Name:      echo.Events,
	example.Name:   example.Events,
	panicstep.Name: panicstep.Events,
	noreturn.Name:  noreturn.Events,
	hanging.Name:   hanging.Events,
	channels.Name:  channels.Events,
	cmd.Name:       cmd.Events,
	crash.Name:     crash.Events,
	fail.Name:      fail.Events,
}

func TestMain(m *testing.M) {
	logging.GetLogger("tests/integ")
	logging.Disable()

	pluginRegistry = pluginregistry.NewPluginRegistry()
	// Setup the PluginRegistry by registering TestSteps
	for name, tsfactory := range testSteps {
		if _, ok := testStepsEvents[name]; !ok {
			err := fmt.Errorf("TestStep %s does not define any associated event", name)
			panic(err)
		}
		if err := pluginRegistry.RegisterTestStep(name, tsfactory, testStepsEvents[name]); err != nil {
			panic(fmt.Sprintf("could not register TestStep: %+v", err))
		}
	}
	// Setup test Targets and empty parameters
	targets = []*target.Target{
		&target.Target{ID: "001", FQDN: "host001.facebook.com"},
		&target.Target{ID: "002", FQDN: "host002.facebook.com"},
		&target.Target{ID: "003", FQDN: "host003.facebook.com"},
		&target.Target{ID: "004", FQDN: "host004.facebook.com"},
		&target.Target{ID: "005", FQDN: "host005.facebook.com"},
	}

	// Configure the storage layer to be in-memory for TestRunner integration tests.
	// This layer persists across all tests.
	s, err := memory.New()
	if err != nil {
		panic(fmt.Sprintf("could not initialize in-memory storage layer: %v", err))
	}
	err = storage.SetStorage(s)
	if err != nil {
		panic(fmt.Sprintf("could not set storage memory layer: %v", err))
	}

	os.Exit(m.Run())
}

func TestSuccessfulCompletion(t *testing.T) {

	jobID := types.JobID(1)
	runID := types.RunID(1)

	ts1, err := pluginRegistry.NewTestStep("Example")
	require.NoError(t, err)
	ts2, err := pluginRegistry.NewTestStep("Example")
	require.NoError(t, err)
	ts3, err := pluginRegistry.NewTestStep("Example")
	require.NoError(t, err)

	params := make(test.TestStepParameters)
	testSteps := []test.TestStepBundle{
		test.TestStepBundle{TestStep: ts1, TestStepLabel: "FirstStage", Parameters: params},
		test.TestStepBundle{TestStep: ts2, TestStepLabel: "SecondStage", Parameters: params},
		test.TestStepBundle{TestStep: ts3, TestStepLabel: "ThirdStage", Parameters: params},
	}

	errCh := make(chan error, 1)
	stateCtx, _, _ := xcontext.New()

	go func() {
		tr := runner.NewTestRunner()
		_, err := tr.Run(stateCtx, &test.Test{TestStepsBundles: testSteps}, targets, jobID, runID, nil)
		errCh <- err
	}()
	select {
	case err = <-errCh:
		require.NoError(t, err)
	case <-time.After(successTimeout):
		t.Errorf("test should return within timeout (%s)", successTimeout.String())
	}
}

func TestCmdPlugin(t *testing.T) {

	jobID := types.JobID(1)
	runID := types.RunID(1)

	ts1, err := pluginRegistry.NewTestStep("cmd")
	require.NoError(t, err)

	params := make(test.TestStepParameters)
	params["executable"] = []test.Param{
		*test.NewParam("sleep"),
	}
	params["args"] = []test.Param{
		*test.NewParam("5"),
	}

	testSteps := []test.TestStepBundle{
		test.TestStepBundle{TestStep: ts1, Parameters: params},
	}

	stateCtx, _, cancel := xcontext.New()
	errCh := make(chan error, 1)

	go func() {
		tr := runner.NewTestRunner()
		_, err := tr.Run(stateCtx, &test.Test{TestStepsBundles: testSteps}, targets, jobID, runID, nil)
		errCh <- err
	}()

	go func() {
		time.Sleep(time.Second)
		cancel()
	}()

	select {
	case <-errCh:
	case <-time.After(successTimeout):
		t.Errorf("test should return within timeout: %+v", successTimeout)
	}
}
