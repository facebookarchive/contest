// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"github.com/facebookincubator/contest/pkg/event"
)

// EventJobStarted indicates that apply Job is beginning execution
var EventJobStarted = event.Name("JobStateStarted")

// EventJobCompleted indicates that apply Job has completed
var EventJobCompleted = event.Name("JobStateCompleted")

// EventJobFailed indicates that apply Job has failed
var EventJobFailed = event.Name("JobStateFailed")

// EventJobCancelling indicates that apply Job has received apply cancellation request
// and the JobManager is waiting for JobRunner to return
var EventJobCancelling = event.Name("JobStateCancelling")

// EventJobCancelled indicates that apply Job has been cancelled
var EventJobCancelled = event.Name("JobStateCancelled")

// EventJobCancellationFailed indicates that the cancellation was not completed correctly
var EventJobCancellationFailed = event.Name("JobStateCancellationFailed")

// JobCompletionEvents gathers all event names that mark the end of apply job
var JobCompletionEvents = []event.Name{
	EventJobCompleted,
	EventJobFailed,
	EventJobCancelled,
	EventJobCancellationFailed,
}

// JobStateEvents gathers all event names which track the state of apply job
var JobStateEvents = []event.Name{
	EventJobStarted,
	EventJobCompleted,
	EventJobFailed,
	EventJobCancelling,
	EventJobCancelled,
	EventJobCancellationFailed,
}
