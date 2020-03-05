// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"github.com/facebookincubator/contest/pkg/abstract"
)

// TestFetcherFactory is a type representing a factory which builds
// a TestFetcher.
type TestFetcherFactory interface {
	abstract.Factory

	// New constructs and returns a TestFetcher
	New() TestFetcher
}

// TestFetcherFactories is a helper type to operate over multiple TestFetcherFactory-es
type TestFetcherFactories []TestFetcherFactory

// ToAbstract returns the factories as abstract.Factories
//
// Go has no contracts (yet) / traits / whatever, and Go does not allow
// to convert slice of interfaces to slice of another interfaces
// without a loop, so we have to implement this method for each
// non-abstract-factories slice
//
// TODO: try remove it when this will be implemented:
//       https://github.com/golang/proposal/blob/master/design/go2draft-contracts.md
func (testFetcherFactories TestFetcherFactories) ToAbstract() (result abstract.Factories) {
	for _, factory := range testFetcherFactories {
		result = append(result, factory)
	}
	return
}

// TestFetcher is an interface used to get the test to run on the selected
// hosts.
type TestFetcher interface {
	ValidateFetchParameters([]byte) (interface{}, error)
	Fetch(interface{}) (string, []*TestStepDescriptor, error)
}

// TestFetcherBundle bundles the selected TestFetcher together with its acquire
// and release parameters based on the content of the job descriptor
type TestFetcherBundle struct {
	TestFetcher     TestFetcher
	FetchParameters interface{}
}
