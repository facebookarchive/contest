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

// TargetError is used by Steps to indicate that a Target encountered
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

// ErrStepsNeverReturned indicates that one or multiple Steps
//  did not complete when the test terminated or when the pipeline
// received a cancellation or pause signal
type ErrStepsNeverReturned struct {
	StepNames []string
}

// Error returns the error string associated with the error
func (e *ErrStepsNeverReturned) Error() string {
	return fmt.Sprintf("test step [%s] did not return", strings.Join(e.StepNames, ", "))
}

// ErrStepClosedChannels indicates that the test step returned after
// closing its output channels, which constitutes an API violation
type ErrStepClosedChannels struct {
	StepName string
}

// Error returns the error string associated with the error
func (e *ErrStepClosedChannels) Error() string {
	return fmt.Sprintf("test step %v closed output channels (api violation)", e.StepName)
}
