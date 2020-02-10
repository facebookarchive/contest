// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetsuccess

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/lib/comparison"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name defines the name of the reporter used within the plugin registry
var Name = "TargetSuccess"

// RunParameters contains the parameters necessary for the run reporter to
// elaborate the results of the Job
type RunParameters struct {
	SuccessExpression string
}

// FinalParameters contains the parameters necessary for the final reporter to
// elaborate the results of the Job
type FinalParameters struct {
	AverageSuccessExpression string
}

// TargetSuccessReporter implements a reporter which determines whether the Job has been
// successful or not based on the number of Targets which succeeded/failed during
// the various Test runs
type TargetSuccessReporter struct {
}

// TargetSuccessReport wraps a report with target success information.
type TargetSuccessReport struct {
	Message         string
	AchievedSuccess string
	DesiredSuccess  string
}

// ValidateRunParameters validates the parameters for the run reporter
func (ts *TargetSuccessReporter) ValidateRunParameters(params []byte) (interface{}, error) {
	var rp RunParameters
	if err := json.Unmarshal(params, &rp); err != nil {
		return nil, err
	}
	if _, err := comparison.ParseExpression(rp.SuccessExpression); err != nil {
		return nil, fmt.Errorf("could not parse success expression")
	}
	return rp, nil
}

// ValidateFinalParameters validates the parameters for the final reporter
func (ts *TargetSuccessReporter) ValidateFinalParameters(params []byte) (interface{}, error) {
	var fp FinalParameters
	if err := json.Unmarshal(params, &fp); err != nil {
		return nil, err
	}
	if _, err := comparison.ParseExpression(fp.AverageSuccessExpression); err != nil {
		return nil, fmt.Errorf("could not parse average success expression")
	}
	return fp, nil
}

// RunReport calculates the report to be associated with a job run.
func (ts *TargetSuccessReporter) RunReport(cancel <-chan struct{}, parameters interface{}, runNumber uint, result *test.TestResult, ev testevent.Fetcher) (*job.Report, error) {
	reportParameters, ok := parameters.(RunParameters)
	if !ok {
		return nil, fmt.Errorf("report parameters should be of type TargetSuccessParameters")
	}

	if result == nil {
		return nil, fmt.Errorf("test result is empty, cannot calculate success metrics")
	}

	var ignoreList []*target.Target

	// Evaluate the success threshold for every test for which we got a TestResult
	res, err := result.GetResult(reportParameters.SuccessExpression, ignoreList)
	if err != nil {
		return nil, fmt.Errorf("could not evaluate the success on at least one test: %v", err)
	}
	reportData := TargetSuccessReport{
		DesiredSuccess: fmt.Sprintf("%s%s", res.Op, res.RHS),
	}
	if !res.Pass {
		reportData.Message = fmt.Sprintf("Test does not pass success criteria: %s", res.Expr)
		reportData.AchievedSuccess = res.LHS
		return &job.Report{Success: false, ReportTime: time.Now(), Data: reportData}, nil
	}
	reportData.Message = fmt.Sprintf("All tests pass success criteria: %s", res.Expr)
	reportData.AchievedSuccess = res.LHS
	return &job.Report{Success: true, ReportTime: time.Now(), Data: reportData}, nil
}

// FinalReport calculates the final report to be associated to a job.
func (ts *TargetSuccessReporter) FinalReport(cancel <-chan struct{}, parameters interface{}, results []*test.TestResult, ev testevent.Fetcher) (*job.Report, error) {
	return nil, fmt.Errorf("final reporting not implemented yet in %s", Name)
}

// New builds a new TargetSuccessReporter
func New() job.Reporter {
	return &TargetSuccessReporter{}
}

// Load returns the name and factory which are needed to register the Reporter
func Load() (string, job.ReporterFactory) {
	return Name, New
}
