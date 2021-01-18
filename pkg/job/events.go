// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/event"
)

// EventJobStarted indicates that a Job is beginning execution
var EventJobStarted = event.Name("JobStateStarted")

// EventJobCompleted indicates that a Job has completed
var EventJobCompleted = event.Name("JobStateCompleted")

// EventJobFailed indicates that a Job has failed
var EventJobFailed = event.Name("JobStateFailed")

// EventJobPaused indicates that a Job was paused
var EventJobPaused = event.Name("JobStatePaused")

// EventJobPauseFailed indicates that a Job pausing was not completed correctly
var EventJobPauseFailed = event.Name("JobStatePauseFailed")

// EventJobCancelling indicates that a Job has received a cancellation request
// and the JobManager is waiting for JobRunner to return
var EventJobCancelling = event.Name("JobStateCancelling")

// EventJobCancelled indicates that a Job has been cancelled
var EventJobCancelled = event.Name("JobStateCancelled")

// EventJobCancellationFailed indicates that the cancellation was not completed correctly
var EventJobCancellationFailed = event.Name("JobStateCancellationFailed")

// JobCompletionEvents gathers all event names that mark the end of a job
var JobCompletionEvents = []event.Name{
	EventJobCompleted,
	EventJobFailed,
	EventJobCancelled,
	EventJobCancellationFailed,
}

// JobStateEvents gathers all event names which track the state of a job
var JobStateEvents = []event.Name{
	EventJobStarted,
	EventJobCompleted,
	EventJobFailed,
	EventJobPaused,
	EventJobPauseFailed,
	EventJobCancelling,
	EventJobCancelled,
	EventJobCancellationFailed,
}

// States corresponding to events.
var jobStates = []State{
	JobStateStarted,
	JobStateCompleted,
	JobStateFailed,
	JobStatePaused,
	JobStatePauseFailed,
	JobStateCancelling,
	JobStateCancelled,
	JobStateCancellationFailed,
}

func EventNameToJobState(ev event.Name) (State, error) {
	for i, e := range JobStateEvents {
		if e == ev {
			return jobStates[i], nil
		}
	}
	return JobStateUnknown, fmt.Errorf("invalid job state %q", ev)
}
