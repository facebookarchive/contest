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

// LimitsValidator provides methods to validate data size from storage perspective
type LimitsValidator interface {
	ValidateTestName(testName string) error
	ValidateTestStepLabel(testStepLabel string) error
	ValidateEventName(eventName string) error
	ValidateReporterName(reporterName string) error
	ValidateJobName(jobName string) error
	ValidateRequesterName(jobRequester string) error
	ValidateServerID(serverID string) error

	// Target name/id??
}

// Validator is an instance of current storage limits validator
var Validator LimitsValidator = &BasicStorageLimitsValidator{}

// BasicStorageLimitsValidator is a simple LimitsValidator implementation
type BasicStorageLimitsValidator struct{}

// MaxTestNameLen is a max length of test name field
const MaxTestNameLen = 32

// ValidateTestName retruns error if the test name does not match storage limitations
func (v *BasicStorageLimitsValidator) ValidateTestName(testName string) error {
	return v.validate(testName, "Test name", MaxTestNameLen)
}

// MaxTestStepLabelLen is a max length of test step label field
const MaxTestStepLabelLen = 32

// ValidateTestStepLabel retruns error if the test step label does not match storage limitations
func (v *BasicStorageLimitsValidator) ValidateTestStepLabel(testStepLabel string) error {
	return v.validate(testStepLabel, "Test step label", MaxTestStepLabelLen)
}

// MaxJobNameLen is a max length of job name field
const MaxJobNameLen = 32

// ValidateJobName retruns error if the job name does not match storage limitations
func (v *BasicStorageLimitsValidator) ValidateJobName(jobName string) error {
	return v.validate(jobName, "Job name", MaxJobNameLen)
}

// MaxEventNameLen is a max length of event name field
const MaxEventNameLen = 32

// ValidateEventName retruns error if the event name does not match storage limitations
func (v *BasicStorageLimitsValidator) ValidateEventName(eventName string) error {
	return v.validate(eventName, "Event name", MaxEventNameLen)
}

// MaxReporterNameLen is a max length of reporter name field
const MaxReporterNameLen = 32

// ValidateReporterName retruns error if the reporter name does not match storage limitations
func (v *BasicStorageLimitsValidator) ValidateReporterName(reporterName string) error {
	return v.validate(reporterName, "Reporter name", MaxReporterNameLen)
}

// MaxRequesterNameLen is a max length of requester name field
const MaxRequesterNameLen = 32

// ValidateRequesterName retruns error if the requester name does not match storage limitations
func (v *BasicStorageLimitsValidator) ValidateRequesterName(requesterName string) error {
	return v.validate(requesterName, "Requester name", MaxRequesterNameLen)
}

// MaxServerIDLen is a max length of server id field
const MaxServerIDLen = 64

// ValidateServerID retruns error if the ServerID does not match storage limitations
func (v *BasicStorageLimitsValidator) ValidateServerID(serverID string) error {
	return v.validate(serverID, "Server ID", MaxServerIDLen)
}

func (v *BasicStorageLimitsValidator) validate(data string, dataName string, maxDataLen int) error {
	if l := len(data); l > maxDataLen {
		return fmt.Errorf("%s is too long: %d > %d", dataName, l, maxDataLen)
	}
	return nil
}
