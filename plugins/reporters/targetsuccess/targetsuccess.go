// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetsuccess

import (
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/lib/comparison"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

// Name defines the name of the reporter used within the plugin registry
var Name = "TargetSuccess"

// TargetSuccessParameters contains the parameters necessary for the reporter to
// elaborate the results of the Job
type TargetSuccessParameters struct {
	SuccessExpression string
}

// TargetSuccessReporter implements a reporter which determines whether the Job has been
// successful or not based on the number of Targets which succeeded/failed during
// the various Test runs
type TargetSuccessReporter struct {
}

type TargetSuccessReport struct {
	Message         string
	AchievedSuccess string
	DesiredSuccess  string
}

// ValidateParameters validates the parameters for the reporter
func (ts *TargetSuccessReporter) ValidateParameters(params []byte) (interface{}, error) {
	var tsp TargetSuccessParameters
	if err := json.Unmarshal(params, &tsp); err != nil {
		return nil, err
	}
	if _, err := comparison.ParseExpression(tsp.SuccessExpression); err != nil {
		return nil, fmt.Errorf("could not parse success expression")
	}
	return tsp, nil
}

// Report calculates the Report object to be associated with the job
func (ts *TargetSuccessReporter) Report(cancel <-chan struct{}, parameters interface{}, result *test.TestResult, ev testevent.Fetcher) (bool, interface{}, error) {
	reportParameters, ok := parameters.(TargetSuccessParameters)
	if !ok {
		return false, nil, fmt.Errorf("report parameteres should be of type TargetSuccessParameters")
	}

	if result == nil {
		return false, nil, fmt.Errorf("test result is empty, cannot calculate success metrics")
	}

	var ignoreList []*target.Target

	// Evaluate the success threshold for every test for which we got a TestResult
	res, err := result.GetResult(reportParameters.SuccessExpression, ignoreList)
	if err != nil {
		return false, nil, fmt.Errorf("could not evaluate the success on at least one test: %v", err)
	}
	report := TargetSuccessReport{}
	report.DesiredSuccess = fmt.Sprintf("%s%s", res.Op, res.Rhs)
	if !res.Pass {
		report.Message = fmt.Sprintf("Test does not pass success criteria: %s", res.Expr)
		report.AchievedSuccess = res.Lhs
		return false, report, nil
	}
	report.Message = fmt.Sprintf("All tests pass success criteria: %s", res.Expr)
	report.AchievedSuccess = res.Lhs
	return true, report, nil
}

// New builds a new TargetSuccessReporter
func New() job.Reporter {
	return &TargetSuccessReporter{}
}
