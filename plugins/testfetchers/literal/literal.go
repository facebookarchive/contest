// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package literal implements a test fetcher that embeds the test step
// definitions, instead of fetching them.
package literal

import (
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name defined the name of the plugin
var (
	Name = "Literal"
)

// FetchParameters contains the parameters necessary to fetch tests. This
// structure is populated from a JSON blob.
type FetchParameters struct {
	TestName string
	Steps    []*test.TestStepDescriptor
}

// Literal implements contest.TestFetcher interface, returning dummy test fetcher
type Literal struct {
}

// ValidateFetchParameters performs sanity checks on the fields of the
// parameters that will be passed to Fetch.
func (tf Literal) ValidateFetchParameters(_ xcontext.Context, params []byte) (interface{}, error) {
	var fp FetchParameters
	if err := json.Unmarshal(params, &fp); err != nil {
		return nil, err
	}
	if fp.TestName == "" {
		return nil, fmt.Errorf("test name cannot be empty for fetch parameters")
	}
	return fp, nil
}

// Fetch returns the information necessary to build a Test object. The returned
// values are:
// * Name of the test
// * list of step definitions
// * an error if any
func (tf *Literal) Fetch(_ xcontext.Context, params interface{}) (string, []*test.TestStepDescriptor, error) {
	fetchParams, ok := params.(FetchParameters)
	if !ok {
		return "", nil, fmt.Errorf("Fetch expects uri.FetchParameters object")
	}
	return fetchParams.TestName, fetchParams.Steps, nil
}

// New initializes the TestFetcher object
func New() test.TestFetcher {
	return &Literal{}
}

// Load returns the name and factory which are needed to register the
// TestFetcher.
func Load() (string, test.TestFetcherFactory) {
	return Name, New
}
