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

// Name returns the Name of the reporter
func (n *Noop) Name() string {
	return Name
}

// RunReport calculates the report to be associated with a job run.
func (n *Noop) RunReport(cancel <-chan struct{}, parameters interface{}, runStatus *job.RunStatus, ev testevent.Fetcher) (bool, interface{}, error) {
	return true, fmt.Sprintf("I did nothing"), nil
}

// FinalReport calculates the final report to be associated to a job.
func (n *Noop) FinalReport(cancel <-chan struct{}, parameters interface{}, runStatuses []job.RunStatus, ev testevent.Fetcher) (bool, interface{}, error) {
	return false, &job.Report{
		Success:    true,
		ReportTime: time.Now(),
		Data:       "I did nothing at the end, all good",
	}, nil
}

type Factory struct{}

// New builds a new Noop
func (f *Factory) New() job.Reporter {
	return &Noop{}
}

// UniqueImplementationName returns the unique name of the implementation
func (f *Factory) UniqueImplementationName() string {
	return Name
}
