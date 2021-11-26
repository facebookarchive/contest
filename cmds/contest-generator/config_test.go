// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseValidSimpleConfig(t *testing.T) {
	data := []byte(`targetmanagers:
  - path: target/manager/tm1
  - path: target/manager/tm2
testfetchers:
  - path: test/fetchers/tf
teststeps:
  - path: test/steps/ts
reporters:
  - path: reporters/rep
`)
	cfg, err := parseConfig("testparseconfig.yml", data)
	require.NoError(t, err)
	assert.Equal(t, "testparseconfig.yml", cfg.ConfigFile)
	assert.NoError(t, cfg.Validate())
	// validate target managers
	require.Equal(t, 2, len(cfg.TargetManagers))
	assert.Equal(t, "target/manager/tm1", cfg.TargetManagers[0].Path)
	assert.Equal(t, "", cfg.TargetManagers[0].Alias)
	assert.Equal(t, "tm1", cfg.TargetManagers[0].ToAlias())
	assert.NoError(t, cfg.TargetManagers[0].Validate())
	assert.Equal(t, "target/manager/tm2", cfg.TargetManagers[1].Path)
	assert.Equal(t, "", cfg.TargetManagers[1].Alias)
	assert.Equal(t, "tm2", cfg.TargetManagers[1].ToAlias())
	assert.NoError(t, cfg.TargetManagers[1].Validate())
	// validate test fetchers
	require.Equal(t, 1, len(cfg.TestFetchers))
	assert.Equal(t, "test/fetchers/tf", cfg.TestFetchers[0].Path)
	assert.Equal(t, "", cfg.TestFetchers[0].Alias)
	assert.Equal(t, "tf", cfg.TestFetchers[0].ToAlias())
	assert.NoError(t, cfg.TestFetchers[0].Validate())
	// validate test steps
	require.Equal(t, 1, len(cfg.TestSteps))
	assert.Equal(t, "test/steps/ts", cfg.TestSteps[0].Path)
	assert.Equal(t, "", cfg.TestSteps[0].Alias)
	assert.Equal(t, "ts", cfg.TestSteps[0].ToAlias())
	assert.NoError(t, cfg.TestSteps[0].Validate())
	// validate test steps
	require.Equal(t, 1, len(cfg.Reporters))
	assert.Equal(t, "reporters/rep", cfg.Reporters[0].Path)
	assert.Equal(t, "", cfg.Reporters[0].Alias)
	assert.Equal(t, "rep", cfg.Reporters[0].ToAlias())
	assert.NoError(t, cfg.Reporters[0].Validate())
}

func TestParseInvalidConfig(t *testing.T) {
	data := []byte(`targetmanagers:
  - path: target/manager/tm
testfetchers:
teststeps:
`)
	_, err := parseConfig("invalidconfig.yml", data)
	require.Error(t, err)
}

func TestParseInvalidConfigDuplicates(t *testing.T) {
	data := []byte(`targetmanagers:
  - path: target/manager/tm
testfetchers:
  - path: test/fetchers/tf
  - path: test/fetchers/tm # duplicate
teststeps:
  - path: test/steps/ts
reporters:
  - path: reporters/rep
`)
	_, err := parseConfig("invalidconfigduplicate.yml", data)
	require.Error(t, err)
}

func TestParseValidConfigDuplicatesWithAlias(t *testing.T) {
	data := []byte(`targetmanagers:
  - path: target/manager/tm
testfetchers:
  - path: test/fetchers/tf
  - path: test/fetchers/tm # duplicate but with alias
    alias: tm2
teststeps:
  - path: test/steps/ts
reporters:
  - path: reporters/rep
`)
	cfg, err := parseConfig("invalidconfigduplicate.yml", data)
	require.NoError(t, err)
	assert.Equal(t, 1, len(cfg.TargetManagers))
	assert.Equal(t, 2, len(cfg.TestFetchers))
	assert.Equal(t, "tm", cfg.TargetManagers[0].ToAlias())
	assert.Equal(t, "tm2", cfg.TestFetchers[1].ToAlias())
}
