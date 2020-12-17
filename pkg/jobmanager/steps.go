// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/test"
)

// stepsResolver is an interface which determines how to fetch TestStepsDescriptors, which could
// have either already been pre-calculated, or built by the TestFetcher.
type stepsResolver interface {
	GetStepsDescriptors() ([]test.TestStepsDescriptors, error)
}

type literalStepsResolver struct {
	stepsDescriptors []test.TestStepsDescriptors
}

func (l literalStepsResolver) GetStepsDescriptors() ([]test.TestStepsDescriptors, error) {
	return l.stepsDescriptors, nil
}

type fetcherStepsResolver struct {
	jobDescriptor *job.Descriptor
	registry      *pluginregistry.PluginRegistry
}

func (f fetcherStepsResolver) GetStepsDescriptors() ([]test.TestStepsDescriptors, error) {

	var descriptors []test.TestStepsDescriptors
	for _, testDescriptor := range f.jobDescriptor.TestDescriptors {
		bundleTestFetcher, err := f.registry.NewTestFetcherBundle(testDescriptor)
		if err != nil {
			return nil, err
		}
		testName, stepDescriptors, err := bundleTestFetcher.TestFetcher.Fetch(bundleTestFetcher.FetchParameters)

		if err != nil {
			return nil, err
		}
		descriptors = append(descriptors, test.TestStepsDescriptors{TestName: testName, TestSteps: stepDescriptors})
	}
	return descriptors, nil
}
