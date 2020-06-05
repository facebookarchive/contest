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

// stepsResolver is an interface which determines how to fetch StepsDescriptors, which could
// have either already been pre-calcualted, or built by the TestFetcher.
type stepsResolver interface {
	GetStepsDescriptors() ([]test.StepsDescriptors, error)
}

type literalStepsResolver struct {
	stepsDescriptors []test.StepsDescriptors
}

func (l literalStepsResolver) GetStepsDescriptors() ([]test.StepsDescriptors, error) {
	return l.stepsDescriptors, nil
}

type fetcherStepsResolver struct {
	jobDescriptor *job.Descriptor
	registry      *pluginregistry.PluginRegistry
}

func (f fetcherStepsResolver) GetStepsDescriptors() ([]test.StepsDescriptors, error) {

	var descriptors []test.StepsDescriptors
	for _, testDescriptor := range f.jobDescriptor.TestDescriptors {
		bundleTestFetcher, err := f.registry.NewTestFetcherBundle(&testDescriptor)
		if err != nil {
			return nil, err
		}
		d, err := bundleTestFetcher.TestFetcher.Fetch(bundleTestFetcher.FetchParameters)
		if err != nil {
			return nil, err
		}
		descriptors = append(descriptors, *d)
	}
	return descriptors, nil
}
