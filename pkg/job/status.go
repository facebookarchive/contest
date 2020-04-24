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
// (jobID, runID, testName)                -> []TestStepStatus [within a test there might be multiple steps]
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

// TestStepCoordinates collects information to identify the test step for which we want to rebuild the status
type TestStepCoordinates struct {
	TestCoordinates
	TestStepName  string
	TestStepLabel string
}

// TargetStatus represents the status of a Target within a TestStep
type TargetStatus struct {
	TestStepCoordinates
	Target  *target.Target
	InTime  time.Time
	OutTime time.Time
	Error   string
}

// TestStepStatus bundles together all the TargetStatus for a specific TestStep (represented via
// its name and label)
type TestStepStatus struct {
	TestStepCoordinates
	Events         []testevent.Event
	TargetStatuses []TargetStatus
}

// TestStatus bundles together all TestStepStatus for a specific Test within the run
type TestStatus struct {
	TestCoordinates
	TestStepStatuses []TestStepStatus
	TargetStatuses   []TargetStatus
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
