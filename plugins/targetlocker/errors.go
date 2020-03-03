// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetlocker

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// ErrNonPositiveTimeout means the timeout value (passed as the argument
// to New) is too small to be used properly. Consider increasing this
// value to be at least time.Second.
type ErrNonPositiveTimeout struct {
	Timeout time.Duration
}

func (err ErrNonPositiveTimeout) Error() string {
	return fmt.Sprintf("non positive (or too small) timeout: %v", err.Timeout)
}

// ErrAlreadyLocked means the target `Target` is already locked by another job.
type ErrAlreadyLocked struct {
	TargetID        string
	UnderlyingError error
}

func (err ErrAlreadyLocked) Error() string {
	return fmt.Sprintf("target %s is already locked, underlying error: %v",
		err.TargetID, err.UnderlyingError)
}
func (err ErrAlreadyLocked) Unwrap() error {
	return err.UnderlyingError
}

// ErrUnableToPerformAction means the requested action wasn't performed
// successfully. The underlying reason is included in `Err`.
type ErrUnableToPerformAction struct {
	Action Action
	JobID  types.JobID
	Target *target.Target
	Err    error
}

func (err ErrUnableToPerformAction) Error() string {
	switch err.Err {
	case sql.ErrNoRows:
		if err.JobID == 0 || err.Action == ActionLock {
			break
		}
		return fmt.Sprintf("unable to perform '%s' on target '%s': no lock (owned by job %d) was found: %v",
			err.Action, err.Target, err.JobID, err.Err)
	}
	return fmt.Sprintf("unable to perform '%s' on target '%s': %v", err.Action, err.Target, err.Err)
}
func (err ErrUnableToPerformAction) Unwrap() error {
	return err.Err
}

// ErrInvalidAction is an internal error of a target.Locker implementation.
// It means that the value of `Action` was not expected.
type ErrInvalidAction struct {
	Action Action
}

func (err ErrInvalidAction) Error() string {
	return fmt.Sprintf("invalid action: %s", err.Action)
}

// ErrUnableToGetLocksInfo is used when it wasn't unable to get information
// about locks for requested targets. See underlying reason in `Err`.
type ErrUnableToGetLocksInfo struct {
	Err error
}

func (err ErrUnableToGetLocksInfo) Error() string {
	return fmt.Sprintf("unable to get information about locks, error: %v", err.Err)
}
func (err ErrUnableToGetLocksInfo) Unwrap() error {
	return err.Err
}

// ErrInvalidDSN means the DSN parsed as the second argument to New() wasn't
// successfully parsed.
type ErrInvalidDSN struct {
	Err error
}

func (err ErrInvalidDSN) Error() string {
	return fmt.Sprintf("invalid DSN: %v", err.Err)
}
func (err ErrInvalidDSN) Unwrap() error {
	return err.Err
}
