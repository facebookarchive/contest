// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// The hierarchy of status objects is the following
// (jobID,)                                -> []RunStatus [within a job, there might be multiple runs]
// (jobID, runID)                          -> []TestStatus [within a run, there might be multiple tests]
// (jobID, runID, testName)                -> []StepStatus [within a test there might be multiple steps]
// (jobID, runID, testName, testStepLabel) -> []TargetStepStatus [within a step, multiple targets have been tested]

// RunCoordinates collects information to identify the run for which we want to rebuild the status
type RunCoordinates struct {
	JobID types.JobID
	RunID types.RunID
}

// TestCoordinates collects information to identify the test for which we want to rebuild the status
type TestCoordinates struct {
	RunCoordinates
	TestName string
}

// StepCoordinates collects information to identify the test step for which we want to rebuild the status
type StepCoordinates struct {
	TestCoordinates
	StepName  string
	StepLabel string
}

// TargetStatus represents the status of a Target within a Step
type TargetStatus struct {
	StepCoordinates
	Target  *target.Target
	InTime  time.Time
	OutTime time.Time
	Error   string
	// these are events that have an associated target. For events
	// that are not associated to a target, see StepStatus.Events .
	Events []testevent.Event
}

// StepStatus bundles together all the TargetStatus for a specific Step (represented via
// its name and label)
type StepStatus struct {
	StepCoordinates
	// these are events that have no target associated. For events
	// associated to a target, see TargetStatus.TargetEvents .
	Events         []testevent.Event
	TargetStatuses []TargetStatus
}

// TestStatus bundles together all StepStatus for a specific Test within the run
type TestStatus struct {
	TestCoordinates
	StepStatuses   []StepStatus
	TargetStatuses []TargetStatus
}

// RunStatus bundles together all TestStatus for a specific run within the job
type RunStatus struct {
	RunCoordinates
	StartTime    time.Time
	TestStatuses []TestStatus
}

// Status contains information about a job's current status which is conveyed
// via the API when answering Status requests
type Status struct {
	// Name is the name of the job.
	Name string

	// State represents the last recorded state of a job
	State string

	// StateErrMsg is an optional error message associated to the job state
	StateErrMsg string

	// StartTime indicates when the job started. A value of 0 indicates "not
	// started yet"
	StartTime time.Time

	// EndTime indicates when the job ended.
	EndTime *time.Time

	// RunStatuses represents the status of the current run of the job
	RunStatus RunStatus

	// Job report information
	JobReport *JobReport
}
