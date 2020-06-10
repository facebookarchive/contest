// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"fmt"
)

// ErrTestRunnerError represents an error returned by the test runner, which might embed both an
// error coming from the
type ErrTestRunnerError struct {
	errTestPipeline    error
	errCleanupPipeline error
}

func (e ErrTestRunnerError) Error() string {
	var err string
	if e.errTestPipeline != nil {
		err = fmt.Sprintf("test pipeline failed: %s", e.errTestPipeline.Error())
	}
	if e.errCleanupPipeline != nil {
		err = fmt.Sprintf("cleanup pipeline failed: %s", e.errCleanupPipeline.Error())
	}
	return err
}

func newErrTestRunnerError(errTestPipeline error, errCleanupPipeline error) error {
	if errTestPipeline == nil && errCleanupPipeline == nil {
		return nil
	}
	return ErrTestRunnerError{errTestPipeline: errTestPipeline, errCleanupPipeline: errCleanupPipeline}
}

// ErrTerminationRequested is an error which indicates that a termination signal has been asserted
type ErrTerminationRequested struct {
}

func (e *ErrTerminationRequested) Error() string {
	return "termination requested"
}
