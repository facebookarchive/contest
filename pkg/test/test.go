// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"encoding/json"
	"errors"

	"github.com/facebookincubator/contest/pkg/target"
)

// Test describes a test definition.
type Test struct {
	Name string

	TestStepsBundles    []StepBundle
	CleanupStepsBundles []StepBundle

	TargetManagerBundle *target.TargetManagerBundle
	TestFetcherBundle   *TestFetcherBundle
}

// Descriptor models the JSON encoded text which is given as input to the
// job creation request. The test descriptors are part of the main JobDescriptor
// JSON document.
type Descriptor struct {
	// TargetManager-related parameters
	TargetManagerName              string
	TargetManagerAcquireParameters json.RawMessage
	TargetManagerReleaseParameters json.RawMessage

	// TestFetcher-related parameters
	TestFetcherName            string
	TestFetcherFetchParameters json.RawMessage
}

// Validate performs sanity checks on the Descriptor
func (d *Descriptor) Validate() error {
	if d.TargetManagerName == "" {
		return errors.New("target manager name cannot be empty")
	}
	if d.TestFetcherName == "" {
		return errors.New("test fetcher name cannot be empty")
	}
	return nil
}
