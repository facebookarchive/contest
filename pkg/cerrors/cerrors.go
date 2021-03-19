// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package cerrors

import (
	"fmt"
	"strings"

	"github.com/facebookincubator/contest/pkg/target"
)

// TargetError is used by TestSteps to indicate that a Target encountered
// an error while running the test. A Target that encounters a TargetError
// does not proceed further in the test run.
type TargetError struct {
	Target *target.Target
	Err    error
}

// ErrResumeNotSupported indicates that a test step cannot resume. This can
// be checked explicitly by the framework
type ErrResumeNotSupported struct {
	StepName string
}

// Error returns the error string associated with the error
func (e *ErrResumeNotSupported) Error() string {
	return fmt.Sprintf("test step %s does not support resume", e.StepName)
}

// ErrTestStepsNeverReturned indicates that one or multiple TestSteps
//  did not complete when the test terminated or when the pipeline
// received a cancellation or pause signal
type ErrTestStepsNeverReturned struct {
	StepNames []string
}

// Error returns the error string associated with the error
func (e *ErrTestStepsNeverReturned) Error() string {
	return fmt.Sprintf("test step [%s] did not return", strings.Join(e.StepNames, ", "))
}

// ErrTestStepClosedChannels indicates that the test step returned after
// closing its output channels, which constitutes an API violation
type ErrTestStepClosedChannels struct {
	StepName string
}

// Error returns the error string associated with the error
func (e *ErrTestStepClosedChannels) Error() string {
	return fmt.Sprintf("test step %v closed output channels (api violation)", e.StepName)
}

// ErrTestStepPaniced indicates that a test step's method panicked.
type ErrTestStepPaniced struct {
	StepName   string
	StackTrace string
}

// Error returns the error string associated with the error
func (e *ErrTestStepPaniced) Error() string {
	return fmt.Sprintf("test step %s paniced, trace: %q", e.StepName, e.StackTrace)
}

// ErrTestStepReturnedDuplicateResult indicates that a test step returned result
// twice for the same target.
type ErrTestStepReturnedDuplicateResult struct {
	StepName string
	Target   string
}

// Error returns the error string associated with the error
func (e *ErrTestStepReturnedDuplicateResult) Error() string {
	return fmt.Sprintf("test step %s returned duplicate result for %s", e.StepName, e.Target)
}

// ErrTestStepReturnedUnexpectedResult indicates that a test step returned result
// for a target that was not given to it.
type ErrTestStepReturnedUnexpectedResult struct {
	StepName string
	Target   string
}

// Error returns the error string associated with the error
func (e *ErrTestStepReturnedUnexpectedResult) Error() string {
	return fmt.Sprintf("test step %s returned unexpected result for %s", e.StepName, e.Target)
}

// ErrTestStepLostTargets indicates that targets have been lost during test run.
type ErrTestStepLostTargets struct {
	StepName string
	Targets  []string
}

// Error returns the error string associated with the error
func (e *ErrTestStepLostTargets) Error() string {
	return fmt.Sprintf("test step %s lost targets %v", e.StepName, e.Targets)
}
