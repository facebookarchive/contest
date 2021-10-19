// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/runner"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mu        sync.Mutex
	currentID int
)

func nextID() int {
	mu.Lock()
	defer mu.Unlock()

	currentID++
	return currentID
}

func runExecPlugin(t *testing.T, ctx xcontext.Context, jsonParams string) error {
	jobID := types.JobID(nextID())
	runID := types.RunID(1)

	ts, err := pluginRegistry.NewTestStep("exec")
	require.NoError(t, err)

	params := make(test.TestStepParameters)
	params["bag"] = []test.Param{
		*test.NewParam(jsonParams),
	}

	testSteps := []test.TestStepBundle{
		{TestStep: ts, Parameters: params},
	}

	errCh := make(chan error, 1)

	go func() {
		defer func() {
			if e := recover(); e != nil {
				errCh <- e.(error)
			}
			errCh <- nil
		}()

		tr := runner.NewTestRunner()
		_, err := tr.Run(ctx, &test.Test{TestStepsBundles: testSteps}, targets, jobID, runID, nil)
		if err != nil {
			panic(err)
		}

		ev := storage.NewTestEventFetcher()
		events, err := ev.Fetch(ctx, testevent.QueryJobID(jobID), testevent.QueryEventName(target.EventTargetErr))
		if err != nil {
			panic(err)
		}

		if events != nil {
			var payload struct {
				Error string
			}

			if err := json.Unmarshal(*events[0].Data.Payload, &payload); err != nil {
				panic(err)
			}
			panic(fmt.Errorf("step error: %s", payload.Error))
		}
	}()

	select {
	case err := <-errCh:
		return err

	case <-time.After(successTimeout):
		t.Errorf("test should return within timeout: %+v", successTimeout)
	}

	return nil
}

func TestExecPluginLocalSimple(t *testing.T) {
	jsonParams := `
	{
		"bin": {
			"path": "/bin/sh",
			"args": [
				"-c",
				"echo 42"
			]
		},
		"transport": {
			"proto": "local"
		}
	}`
	if err := runExecPlugin(t, ctx, jsonParams); err != nil {
		t.Error(err)
	}
}

func TestExecPluginLocalTimeout(t *testing.T) {
	jsonParams := `
	{
		"bin": {
			"path": "/bin/sleep",
			"args": [
				"20"
			]
		},
		"transport": {
			"proto": "local"
		},
		"constraints": {
			"time_quota": "1s"
		}
	}`
	err := runExecPlugin(t, ctx, jsonParams)
	if !strings.Contains(err.Error(), "killed") {
		t.Error(err)
	}
}

func TestExecPluginSSHSimple(t *testing.T) {
	jsonParams := `
	{
		"bin": {
			"path": "/bin/sh",
			"args": [
				"-c",
				"echo 42"
			]
		},
		"transport": {
			"proto": "ssh",
			"options": {
				"host": "localhost",
				"user": "root",
				"identity_file": "/root/.ssh/id_rsa"
			}
		}
	}`
	if err := runExecPlugin(t, ctx, jsonParams); err != nil {
		t.Error(err)
	}
}

func TestExecPluginSSHAsync(t *testing.T) {
	jsonParams := `
	{
		"bin": {
			"path": "/bin/sh",
			"args": [
				"-c",
				"echo 42"
			]
		},
		"transport": {
			"proto": "ssh",
			"options": {
				"host": "localhost",
				"user": "root",
				"identity_file": "/root/.ssh/id_rsa",
				"send_binary": true,
				"async": {
					"agent": "/go/src/github.com/facebookincubator/contest/exec_agent",
					"time_quota": "20s"
				}
			}
		}
	}`
	if err := runExecPlugin(t, ctx, jsonParams); err != nil {
		t.Error(err)
	}
}

func TestExecPluginLocalCancel(t *testing.T) {
	jsonParams := `
	{
		"bin": {
			"path": "/bin/sleep",
			"args": [
				"20"
			]
		},
		"transport": {
			"proto": "local"
		}
	}`

	ctx, cancel := xcontext.WithTimeout(ctx, time.Second)
	defer cancel()

	err := runExecPlugin(t, ctx, jsonParams)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExecPluginSSHCancel(t *testing.T) {
	jsonParams := `
	{
		"bin": {
			"path": "/bin/sleep",
			"args": [
				"20"
			]
		},
		"transport": {
			"proto": "ssh",
			"options": {
				"host": "localhost",
				"user": "root",
				"identity_file": "/root/.ssh/id_rsa"
			}
		}
	}`

	ctx, cancel := xcontext.WithTimeout(ctx, time.Second)
	defer cancel()

	err := runExecPlugin(t, ctx, jsonParams)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExecPluginSSHAsyncCancel(t *testing.T) {
	jsonParams := `
	{
		"bin": {
			"path": "/bin/sleep",
			"args": [
				"20"
			]
		},
		"transport": {
			"proto": "ssh",
			"options": {
				"host": "localhost",
				"user": "root",
				"identity_file": "/root/.ssh/id_rsa",
				"send_binary": true,
				"async": {
					"agent": "/go/src/github.com/facebookincubator/contest/exec_agent",
					"time_quota": "25s"
				}
			}
		}
	}`

	ctx, cancel := xcontext.WithTimeout(ctx, time.Second)
	defer cancel()

	err := runExecPlugin(t, ctx, jsonParams)
	assert.ErrorIs(t, err, context.Canceled)
}
