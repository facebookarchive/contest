// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetsuccess

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

// nolint:unused
type TargetSuccessSuite struct {
	suite.Suite
	sampleTestResult test.TestResult
}

func (suite *TargetSuccessSuite) SetupTest() {
	suite.sampleTestResult = test.NewTestResult(types.JobID(1))
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target001", ID: "0001", FQDN: "Target001.facebook.com"}, fmt.Errorf("Target001 failed"))
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target002", ID: "0001", FQDN: "Target002.facebook.com"}, nil)
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target003", ID: "0001", FQDN: "Target003.facebook.com"}, nil)
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target004", ID: "0001", FQDN: "Target004.facebook.com"}, nil)
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target005", ID: "0001", FQDN: "Target005.facebook.com"}, nil)
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target006", ID: "0001", FQDN: "Target006.facebook.com"}, fmt.Errorf("Target006 failed"))
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target007", ID: "0001", FQDN: "Target007.facebook.com"}, fmt.Errorf("Target007 failed"))
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target008", ID: "0001", FQDN: "Target008.facebook.com"}, fmt.Errorf("Target008 failed"))
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target009", ID: "0001", FQDN: "Target009.facebook.com"}, fmt.Errorf("Target009 failed"))
	suite.sampleTestResult.SetTarget(&target.Target{Name: "Target010", ID: "0001", FQDN: "Target010.facebook.com"}, nil)
}

func (suite *TargetSuccessSuite) TestTargetSuccessValidateRunParametersInvalidExpression() {
	expr := "#$1"
	tsp := RunParameters{SuccessExpression: expr}
	b, err := json.Marshal(tsp)
	if err != nil {
		suite.T().Errorf("could not marshal TargetSuccessParameters: %v", err)
	}
	tsr := TargetSuccessReporter{}
	_, err = tsr.ValidateRunParameters(b)
	if err == nil {
		suite.T().Errorf("expression %s should be rejected as invalid, but it wasn't", expr)
	}
}

func (suite *TargetSuccessSuite) TestTargetSuccessValidateRunParametersValidExpression() {
	expr := ">1"
	tsp := RunParameters{SuccessExpression: expr}
	b, err := json.Marshal(tsp)
	if err != nil {
		suite.T().Errorf("could not marshal TargetSuccessParameters: %v", err)
	}
	tsr := TargetSuccessReporter{}
	_, err = tsr.ValidateRunParameters(b)
	if err != nil {
		suite.T().Errorf("expression %s should not be rejected: %v", expr, err)
	}
}

func (suite *TargetSuccessSuite) TestTargetSuccessSuccessfulJob() {
	cancel := make(chan struct{})
	expr := ">40%"
	tsp := RunParameters{SuccessExpression: expr}
	tsr := TargetSuccessReporter{}

	ev := storage.NewTestEventFetcher()
	runNum := uint(1)
	report, err := tsr.RunReport(cancel, tsp, runNum, &suite.sampleTestResult, ev)
	if err != nil {
		suite.T().Errorf("reporting should not fail: %v", err)
	}
	if report.Success != true {
		suite.T().Errorf("report should say that the job is successful")
	}
}

func (suite *TargetSuccessSuite) TestTargetSuccessFailedJob(t *testing.T) {
	cancel := make(chan struct{})
	expr := ">80%"
	tsp := RunParameters{SuccessExpression: expr}
	tsr := TargetSuccessReporter{}

	ev := storage.NewTestEventFetcher()
	runNum := uint(1)
	report, err := tsr.RunReport(cancel, tsp, runNum, &suite.sampleTestResult, ev)
	if err != nil {
		suite.T().Errorf("reporting should not fail: %v", err)
	}
	if report.Success != false {
		suite.T().Errorf("report should say that the job is failed")
	}
}
