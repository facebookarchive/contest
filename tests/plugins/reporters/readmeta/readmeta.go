// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package readmeta_test

import (
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name defines the name of the reporter used within the plugin registry
var Name = "readmeta"

// Noop is a reporter that does nothing. Probably only useful for testing.
type Readmeta struct{}

// ValidateRunParameters validates the parameters for the run reporter
func (n *Readmeta) ValidateRunParameters(params []byte) (interface{}, error) {
	var s string
	return s, nil
}

// ValidateFinalParameters validates the parameters for the final reporter
func (n *Readmeta) ValidateFinalParameters(params []byte) (interface{}, error) {
	var s string
	return s, nil
}

// Name returns the Name of the reporter
func (n *Readmeta) Name() string {
	return Name
}

// RunReport calculates the report to be associated with a job run.
func (n *Readmeta) RunReport(ctx xcontext.Context, parameters interface{}, runStatus *job.RunStatus, ev testevent.Fetcher) (bool, interface{}, error) {
	// test metadata exists
	jobID, ok1 := types.JobIDFromContext(ctx)
	// note this must use panic to abort the test run, as this is a test for the job runner which ignore the actual outcome
	if jobID == 0 || !ok1 {
		panic("Unable to extract jobID from context")
	}
	runID, ok2 := types.RunIDFromContext(ctx)
	if runID == 0 || !ok2 {
		panic("Unable to extract runID from context")
	}
	return true, "I did nothing", nil
}

// FinalReport calculates the final report to be associated to a job.
func (n *Readmeta) FinalReport(ctx xcontext.Context, parameters interface{}, runStatuses []job.RunStatus, ev testevent.Fetcher) (bool, interface{}, error) {
	// this one only has jobID, there is no specific runID in the final reporter
	jobID, ok1 := types.JobIDFromContext(ctx)
	// note this must use panic to abort the test run, as this is a test for the job runner which ignore the actual outcome
	if jobID == 0 || !ok1 {
		panic("Unable to extract jobID from context")
	}
	return true, "I did nothing at all", nil
}

// New builds a new TargetSuccessReporter
func New() job.Reporter {
	return &Readmeta{}
}

// Load returns the name and factory which are needed to register the Reporter
func Load() (string, job.ReporterFactory) {
	return Name, New
}
