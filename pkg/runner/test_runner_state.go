// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

// RunnerState is a structure that models the current state of the test runner
type RunnerState struct {
	completedSteps   map[string]error
	completedRouting map[string]error
	completedTargets map[*target.Target]error
}

func NewRunnerState() *RunnerState {
	r := RunnerState{}
	r.completedSteps = make(map[string]error)
	r.completedRouting = make(map[string]error)
	r.completedTargets = make(map[*target.Target]error)
	return &r
}

// CompletedTargets returns a map that associates each target with its returning error.
// If the target succeeded, the error will be nil
func (r *RunnerState) CompletedTargets() map[*target.Target]error {
	return r.completedTargets
}

// CompletedRouting returns a map that associates each routing block with its returning error.
// If the routing block succeeded, the error will be nil
func (r *RunnerState) CompletedRouting() map[string]error {
	return r.completedRouting
}

// CompletedSteps returns a map that associates each step with its returning error.
// If the step succeeded, the error will be nil
func (r *RunnerState) CompletedSteps() map[string]error {
	return r.completedSteps
}

// SetRouting sets the error associated with a routing block
func (r *RunnerState) SetRouting(testStepLabel string, err error) {
	r.completedRouting[testStepLabel] = err
}

// SetTarget sets the error associated with a target
func (r *RunnerState) SetTarget(target *target.Target, err error) {
	r.completedTargets[target] = err
}

// SetStep sets the error associated with a step
func (r *RunnerState) SetStep(testStepLabel string, err error) {
	r.completedSteps[testStepLabel] = err
}

// IncompleteSteps returns a slice of step names for which the result hasn't been set yet
func (r *RunnerState) IncompleteSteps(bundles []test.TestStepBundle) []string {
	var incompleteSteps []string
	for _, bundle := range bundles {
		if _, ok := r.completedSteps[bundle.TestStepLabel]; !ok {
			incompleteSteps = append(incompleteSteps, bundle.TestStepLabel)
		}
	}
	return incompleteSteps
}
