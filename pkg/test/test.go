// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"encoding/json"

	"github.com/facebookincubator/contest/pkg/targetmanager"
)

// Test describes a test definition.
type Test struct {
	Name                string
	TestStepsBundles    []TestStepBundle
	TargetManagerBundle *targetmanager.TargetManagerBundle
	TestFetcherBundle   *TestFetcherBundle
}

// TestDescriptor models the JSON encoded blob which is given as input to the
// job creation request. The test descriptors are part of the main JobDescriptor
// JSON document.
type TestDescriptor struct {
	// TargetManager-related parameters
	TargetManagerName              string
	TargetManagerAcquireParameters json.RawMessage
	TargetManagerReleaseParameters json.RawMessage

	// TestFetcher-related parameters
	TestFetcherName            string
	TestFetcherFetchParameters json.RawMessage
}
