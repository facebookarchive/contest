// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"fmt"
	"testing"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TestResultSuite struct {
	suite.Suite
	sampleTestResult test.TestResult
	emptyTestResult  test.TestResult
}

// Scenario represent a test Scenario that either evaluates to true or false
type Scenario struct {
	expression string
	expected   bool
	ignore     []*target.Target
}

func (suite *TestResultSuite) SetupTest() {
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

func (suite *TestResultSuite) runScenario(scenarios []Scenario) {
	for _, scenario := range scenarios {
		comparisonResult, err := suite.sampleTestResult.GetResult(scenario.expression, scenario.ignore)
		require.NoError(suite.T(), err)
		msg := fmt.Sprintf(
			"%v expcted to be %v, but it is %v because condition is not met: %v",
			scenario.expression,
			scenario.expected,
			comparisonResult.Pass,
			comparisonResult.Expr,
		)
		require.Equal(suite.T(), scenario.expected, comparisonResult.Pass, msg)
	}
}

func (suite *TestResultSuite) TestResultGt() {
	var ignoreList []*target.Target
	scenarios := []Scenario{
		Scenario{
			expression: ">4",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: ">6",
			expected:   false,
			ignore:     ignoreList,
		},
	}
	suite.runScenario(scenarios)
}

func (suite *TestResultSuite) TestResultLt() {
	var ignoreList []*target.Target
	scenarios := []Scenario{
		Scenario{
			expression: "<1",
			expected:   false,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "<10",
			expected:   true,
			ignore:     ignoreList,
		},
	}
	suite.runScenario(scenarios)
}

func (suite *TestResultSuite) TestResultLe() {
	var ignoreList []*target.Target
	scenarios := []Scenario{
		Scenario{
			expression: "<=5",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "<=10",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "<=3",
			expected:   false,
			ignore:     ignoreList,
		},
	}
	suite.runScenario(scenarios)
}

func (suite *TestResultSuite) TestResultGe() {
	var ignoreList []*target.Target
	scenarios := []Scenario{
		Scenario{
			expression: ">=1",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: ">=10",
			expected:   false,
			ignore:     ignoreList,
		},
	}
	suite.runScenario(scenarios)
}

func (suite *TestResultSuite) TestResultEq() {
	var ignoreList []*target.Target
	scenarios := []Scenario{
		Scenario{
			expression: "=5",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "=10",
			expected:   false,
			ignore:     ignoreList,
		},
	}
	suite.runScenario(scenarios)
}

func (suite *TestResultSuite) TestResultEqPercentage() {
	var ignoreList []*target.Target
	scenarios := []Scenario{
		Scenario{
			expression: "=50%",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "=60%",
			expected:   false,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "<60%",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: ">10%",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "<80%",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: ">60%",
			expected:   false,
			ignore:     ignoreList,
		},
	}
	suite.runScenario(scenarios)
}

func (suite *TestResultSuite) TestResultWithIgnoreList() {

	var emptyIgnoreList []*target.Target
	ignoreList := []*target.Target{
		&target.Target{Name: "Target001", ID: "0001", FQDN: "Target001.facebook.com"},
		&target.Target{Name: "Target006", ID: "0001", FQDN: "Target006.facebook.com"},
		&target.Target{Name: "Target007", ID: "0001", FQDN: "Target007.facebook.com"},
		&target.Target{Name: "Target008", ID: "0001", FQDN: "Target008.facebook.com"},
		&target.Target{Name: "Target009", ID: "0001", FQDN: "Target009.facebook.com"},
	}

	scenarios := []Scenario{
		Scenario{
			expression: "=100%",
			expected:   true,
			ignore:     ignoreList,
		},
		Scenario{
			expression: "=50%",
			expected:   true,
			ignore:     emptyIgnoreList,
		},
	}
	suite.runScenario(scenarios)
}

func (suite *TestResultSuite) TestEmptyResultReturnsError() {
	var ignoreList []*target.Target
	_, err := suite.emptyTestResult.GetResult(">=80%", ignoreList)
	require.Error(suite.T(), err, "result on empty TestResult should return an error")
}

func (suite *TestResultSuite) TestResultUnknownExpression() {
	var ignoreList []*target.Target
	_, err := suite.sampleTestResult.GetResult("invalid expression", ignoreList)
	require.Error(suite.T(), err, "scanario should fail because expression is invalid")
}

func TestTestResultSuite(t *testing.T) {
	suite.Run(t, new(TestResultSuite))
}
