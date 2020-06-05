// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"errors"
	"fmt"
	"strings"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage/limits"
	"github.com/facebookincubator/contest/pkg/test"
)

// newReportingBundles returns the bundles for the run report and the final report
func newReportingBundles(registry *pluginregistry.PluginRegistry, jobDescriptor *job.Descriptor) ([]*job.ReporterBundle, []*job.ReporterBundle, error) {

	var (
		runReporterBundles   []*job.ReporterBundle
		finalReporterBundles []*job.ReporterBundle
	)

	for _, reporter := range jobDescriptor.Reporting.RunReporters {
		if strings.TrimSpace(reporter.Name) == "" {
			return nil, nil, errors.New("invalid empty or all-whitespace run reporter name")
		}
		if err := limits.NewValidator().ValidateReporterName(reporter.Name); err != nil {
			return nil, nil, err
		}
		bundle, err := registry.NewRunReporterBundle(reporter.Name, reporter.Parameters)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create bundle for run reporter '%s': %v", reporter.Name, err)
		}
		runReporterBundles = append(runReporterBundles, bundle)
	}

	for _, reporter := range jobDescriptor.Reporting.FinalReporters {
		if strings.TrimSpace(reporter.Name) == "" {
			return nil, nil, errors.New("invalid empty or all-whitespace final reporter name")
		}
		if err := limits.NewValidator().ValidateReporterName(reporter.Name); err != nil {
			return nil, nil, err
		}
		bundle, err := registry.NewFinalReporterBundle(reporter.Name, reporter.Parameters)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create bundle for final reporter '%s': %v", reporter.Name, err)
		}
		finalReporterBundles = append(finalReporterBundles, bundle)
	}

	return runReporterBundles, finalReporterBundles, nil
}

func newBundlesFromSteps(descriptors []test.StepDescriptor, registry *pluginregistry.PluginRegistry) ([]test.StepBundle, error) {

	// look up test step plugins in the plugin registry
	var stepBundles []test.StepBundle

	for idx, descriptor := range descriptors {
		if err := limits.NewValidator().ValidateTestStepLabel(descriptor.Label); err != nil {
			return nil, err
		}
		tse, err := registry.NewStepEvents(descriptor.Name)
		if err != nil {
			return nil, err
		}
		// test step index is incremented by 1 so we can use 0 to signal an anomaly
		tsb, err := registry.NewStepBundle(&descriptor, uint(idx)+1, tse)
		if err != nil {
			return nil, fmt.Errorf("NewStepBundle for test step '%s' with index %d failed: %w", descriptor.Name, idx, err)
		}
		if err != nil {
			return nil, err
		}
		stepBundles = append(stepBundles, *tsb)
	}

	return stepBundles, nil

}

// newStepBundles creates bundles for test and cleanup steps
func newStepBundles(descriptors test.StepsDescriptors, registry *pluginregistry.PluginRegistry) ([]test.StepBundle, []test.StepBundle, error) {

	testStepBundles, err := newBundlesFromSteps(descriptors.Test, registry)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create test steps bundle: %w", err)
	}
	cleanupStepBundles, err := newBundlesFromSteps(descriptors.Cleanup, registry)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create cleanup steps bundle: %w", err)
	}

	// verify that there are not duplicated labels
	labels := make(map[string]bool)
	for _, bundle := range append(testStepBundles, cleanupStepBundles...) {
		if _, ok := labels[bundle.StepLabel]; ok {
			return nil, nil, fmt.Errorf("duplicated label %s", bundle.StepLabel)
		}
		labels[bundle.StepLabel] = true
	}
	return testStepBundles, cleanupStepBundles, nil
}
