// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package limits

import "fmt"

// Any storage is limited, so we have to be sure that data we are going to store would fit underlaying storage structures
// We could delegate this job to a specific storage plugin, but in this case it has to know too much about job/test descriptor etc
// Another approach is just fail on insertion of invalid data, but we have to try our best in prevention of this situation
// (Image a test failing after multiple hours with a error like "you step label is too long!") So as a tradoff we will try
// to establish and enforce some system wide limits, and make storage plugin developer's life easier

// ErrParameterIsTooLong implements "error" for generic length check error.
type ErrParameterIsTooLong struct {
	DataName  string
	MaxLen    int
	ActualLen int
}

func (err ErrParameterIsTooLong) Error() string {
	return fmt.Sprintf("%s is too long: %d > %d", err.DataName, err.ActualLen, err.MaxLen)
}

// NewValidator returns new instance of storage Validator
func NewValidator() *Validator {
	return &Validator{}
}

// Validator provides methods to validate data structures against storage limitations
type Validator struct{}

// MaxTestNameLen is a max length of test name field
const MaxTestNameLen = 32

// ValidateTestName retruns error if the test name does not match storage limitations
func (v *Validator) ValidateTestName(testName string) error {
	return v.validate(testName, "Test name", MaxTestNameLen)
}

// MaxTestStepLabelLen is a max length of test step label field
const MaxTestStepLabelLen = 32

// ValidateTestStepLabel retruns error if the test step label does not match storage limitations
func (v *Validator) ValidateTestStepLabel(testStepLabel string) error {
	return v.validate(testStepLabel, "Test step label", MaxTestStepLabelLen)
}

// MaxJobNameLen is a max length of job name field
const MaxJobNameLen = 32

// ValidateJobName retruns error if the job name does not match storage limitations
func (v *Validator) ValidateJobName(jobName string) error {
	return v.validate(jobName, "Job name", MaxJobNameLen)
}

// MaxEventNameLen is a max length of event name field
const MaxEventNameLen = 32

// ValidateEventName retruns error if the event name does not match storage limitations
func (v *Validator) ValidateEventName(eventName string) error {
	return v.validate(eventName, "Event name", MaxEventNameLen)
}

// MaxReporterNameLen is a max length of reporter name field
const MaxReporterNameLen = 32

// ValidateReporterName retruns error if the reporter name does not match storage limitations
func (v *Validator) ValidateReporterName(reporterName string) error {
	return v.validate(reporterName, "Reporter name", MaxReporterNameLen)
}

// MaxRequestorNameLen is a max length of Requestor name field
const MaxRequestorNameLen = 32

// ValidateRequestorName retruns error if the requestor name does not match storage limitations
func (v *Validator) ValidateRequestorName(requestorName string) error {
	return v.validate(requestorName, "Requestor name", MaxRequestorNameLen)
}

// MaxServerIDLen is a max length of server id field
const MaxServerIDLen = 64

// ValidateServerID retruns error if the ServerID does not match storage limitations
func (v *Validator) ValidateServerID(serverID string) error {
	return v.validate(serverID, "Server ID", MaxServerIDLen)
}

func (v *Validator) validate(data string, dataName string, maxDataLen int) error {
	if l := len(data); l > maxDataLen {
		return ErrParameterIsTooLong{DataName: dataName, MaxLen: maxDataLen, ActualLen: l}
	}
	return nil
}
