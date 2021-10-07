// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration

package tests

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/runner"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/stretchr/testify/require"
)

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
		{TestStep: ts1, Parameters: params},
	}

	stateCtx, cancel := xcontext.WithCancel(ctx)
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
