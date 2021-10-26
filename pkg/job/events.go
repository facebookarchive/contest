// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
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

// PauseEventPayload is the payload of the JobStatePaused event.
// It is persisted in the database and used to resume jobs.
type PauseEventPayload struct {
	Version int         `json:"V"`
	JobID   types.JobID `json:"J"`
	RunID   types.RunID `json:"R"`
	TestID  int         `json:"T,omitempty"`
	// If we are sleeping before the run, this will specify when the run should begin.
	StartAt *time.Time `json:"S,omitempty"`
	// Otherwise, if test execution is in progress targets and runner state will be populated.
	Targets         []*target.Target `json:"TT,omitempty"`
	TestRunnerState json.RawMessage  `json:"TRS,omitempty"`
}

func (pp *PauseEventPayload) String() string {
	var sts int64
	if pp.StartAt != nil {
		sts = pp.StartAt.Unix()
	}
	return fmt.Sprintf("[V:%d J:%d R:%d T:%d ST:%d TT:%v TRS:%s]",
		pp.Version, pp.JobID, pp.RunID, pp.TestID, sts, pp.Targets, pp.TestRunnerState,
	)
}

// Currently supported version of the pause state.
// Attempting to resume paused jobs with version other than this will fail.
var CurrentPauseEventPayloadVersion = 1
