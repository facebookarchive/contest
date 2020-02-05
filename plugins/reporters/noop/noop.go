// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package noop

import (
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name defines the name of the reporter used within the plugin registry
var Name = "noop"

// Noop is a reporter that does nothing. Probably only useful for testing.
type Noop struct{}

// ValidateRunParameters validates the parameters for the run reporter
func (n *Noop) ValidateRunParameters(params []byte) (interface{}, error) {
	var s string
	return s, nil
}

// ValidateFinalParameters validates the parameters for the final reporter
func (n *Noop) ValidateFinalParameters(params []byte) (interface{}, error) {
	var s string
	return s, nil
}

// RunReport calculates the report to be associated with a job run.
func (n *Noop) RunReport(cancel <-chan struct{}, parameters interface{}, runNumber uint, result *test.TestResult, ev testevent.Fetcher) (*job.Report, error) {
	return &job.Report{
		Success:    true,
		ReportTime: time.Now(),
		Data:       fmt.Sprintf("I did nothing on run #%d, all good", runNumber),
	}, nil
}

// FinalReport calculates the final report to be associated to a job.
func (n *Noop) FinalReport(cancel <-chan struct{}, parameters interface{}, results []*test.TestResult, ev testevent.Fetcher) (*job.Report, error) {
	return &job.Report{
		Success:    true,
		ReportTime: time.Now(),
		Data:       "I did nothing at the end, all good",
	}, nil
}

// New builds a new TargetSuccessReporter
func New() job.Reporter {
	return &Noop{}
}

// Load returns the name and factory which are needed to register the Reporter
func Load() (string, job.ReporterFactory) {
	return Name, New
}
