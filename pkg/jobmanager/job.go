// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"errors"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/test"
)

func newJob(registry *pluginregistry.PluginRegistry, jobDescriptor *job.Descriptor, resolver stepsResolver) (*job.Job, error) {

	if jobDescriptor == nil {
		return nil, errors.New("JobDescriptor cannot be nil")
	}
	if err := jobDescriptor.Validate(); err != nil {
		return nil, fmt.Errorf("could not validate job descriptor: %w", err)
	}
	tests := make([]*test.Test, 0, len(jobDescriptor.TestDescriptors))

	stepsDescriptors, err := resolver.GetStepsDescriptors()
	if err != nil {
		return nil, fmt.Errorf("could not get steps descriptors: %w", err)
	}
	if len(stepsDescriptors) != len(jobDescriptor.TestDescriptors) {
		return nil, fmt.Errorf("length of steps descriptor must match lenght of test descriptors")
	}

	for index, td := range jobDescriptor.TestDescriptors {
		thisTestStepsDescriptors := stepsDescriptors[index]

		if err := td.Validate(); err != nil {
			return nil, fmt.Errorf("could not validate test descriptor: %w", err)
		}
		bundlTargetManager, err := registry.NewTargetManagerBundle(&td)
		if err != nil {
			return nil, err
		}
		bundleTestFetcher, err := registry.NewTestFetcherBundle(&td)
		if err != nil {
			return nil, err
		}

		bundleTest, bundleCleanup, err := newStepsBundles(thisTestStepsDescriptors, registry)
		if err != nil {
			return nil, fmt.Errorf("could not create cleanup step bundles: %v", err)
		}
		test := test.Test{
			Name:                thisTestStepsDescriptors.TestName,
			TargetManagerBundle: bundlTargetManager,
			TestFetcherBundle:   bundleTestFetcher,
			TestStepBundles:     bundleTest,
			CleanupStepBundles:  bundleCleanup,
		}
		tests = append(tests, &test)
	}

	runReportersBundle, finalReportersBundle, err := newReportingBundles(registry, jobDescriptor)
	if err != nil {
		return nil, fmt.Errorf("error while building reporters bundles: %w", err)
	}

	extendedDescriptor := job.ExtendedDescriptor{
		Descriptor:       *jobDescriptor,
		StepsDescriptors: stepsDescriptors,
	}
	// The ID of the job object defaults to zero, and it's populated as soon as the job
	// is persisted in storage.
	job := job.Job{
		ExtendedDescriptor:   &extendedDescriptor,
		Name:                 jobDescriptor.JobName,
		Tags:                 jobDescriptor.Tags,
		Runs:                 jobDescriptor.Runs,
		RunInterval:          time.Duration(jobDescriptor.RunInterval),
		Tests:                tests,
		RunReporterBundles:   runReportersBundle,
		FinalReporterBundles: finalReportersBundle,
	}

	job.Done = make(chan struct{})
	job.CancelCh = make(chan struct{})
	job.PauseCh = make(chan struct{})

	return &job, nil

}

// NewJobFromDescriptor creates a job object from a job descriptor
func NewJobFromDescriptor(registry *pluginregistry.PluginRegistry, jobDescriptor *job.Descriptor) (*job.Job, error) {
	resolver := fetcherStepsResolver{jobDescriptor: jobDescriptor, registry: registry}
	return newJob(registry, jobDescriptor, resolver)
}

// NewJobFromExtendedDescriptor creates a job object from an extended job descriptor
func NewJobFromExtendedDescriptor(registry *pluginregistry.PluginRegistry, jobDescriptor *job.ExtendedDescriptor) (*job.Job, error) {
	resolver := literalStepsResolver{stepsDescriptors: jobDescriptor.StepsDescriptors}
	return newJob(registry, &jobDescriptor.Descriptor, resolver)
}
