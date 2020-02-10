// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"encoding/json"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
)

// TargetStatus bundles the input time and the output time of a target through a TestStep.
type TargetStatus struct {
	Target  *target.Target
	InTime  time.Time
	OutTime time.Time
	Error   *json.RawMessage
}

// TestStepStatus bundles together all the TargetStatus for a specific TestStep (represented via
// its name and label)
type TestStepStatus struct {
	TestStepName  string
	TestStepLabel string
	Events        []testevent.Event
	TargetStatus  []TargetStatus
}

// TestStatus bundles together all TestStepStatus for a specific Test
type TestStatus struct {
	TestName       string
	TestStepStatus []TestStepStatus
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

	// EndTime indicates when the job started. This value should be ignored if
	// `Finished` is false.
	EndTime time.Time

	// TestStatus represents a list of status objects for each test in the job
	TestStatus []TestStatus

	// Job report information
	JobReport *JobReport
}
