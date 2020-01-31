// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"github.com/facebookincubator/contest/pkg/event"
)

// errorPayload represents the payload carried by a failure event (e.g. JobStateFailed, JobStateCancelled, etc.)
type errorPayload struct {
	Err string
}

// EventJobStarted indicates that a Job is beginning execution
var EventJobStarted = event.Name("JobStateStarted")

// EventJobCompleted indicates that a Job has completed
var EventJobCompleted = event.Name("JobStateCompleted")

// EventJobFailed indicates that a Job has failed
var EventJobFailed = event.Name("JobStateFailed")

// EventJobCancelling indicates that a Job has received a cancellation request
// and the JobManager is waiting for JobRunner to return
var EventJobCancelling = event.Name("JobStateCancelling")

// EventJobCancelled indicates that a Job has been cancelled
var EventJobCancelled = event.Name("JobStateCancelled")

// EventJobCancellationFailed indicates that the cancellation was not completed correctly
var EventJobCancellationFailed = event.Name("JobStateCancelled")
