// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package comparison

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Scenario represent a tuple of string expression and expected deserialized Expression object
type Scenario struct {
	expression string
	expected   Expression
}

func runScenario(t *testing.T, scenarios []Scenario) {
	for _, scenario := range scenarios {

		expectedCmp := scenario.expected.Cmp
		expectedType := scenario.expected.Type
		expectedRHS := scenario.expected.RHS

		result, err := ParseExpression(scenario.expression)
		require.NoError(t, err)
		require.Equal(t, expectedType, result.Type, fmt.Sprintf("%s should be of type %v", scenario.expression, expectedType))
		require.Equal(t, expectedCmp.Operator(), result.Cmp.Operator(), fmt.Sprintf("operator should be %v, not %v", expectedCmp.Operator(), result.Cmp.Operator()))
		require.Equal(t, expectedRHS, result.RHS, fmt.Sprintf("rhs should be %.2f, not %.2f", expectedRHS, result.RHS))
	}
}

func TestExpressionGe(t *testing.T) {
	scenarios := []Scenario{
		Scenario{
			expression: ">=80%",
			expected:   Expression{Type: TypePercentage, Cmp: Ge{}, RHS: float64(80)},
		},
		Scenario{
			expression: ">=80",
			expected:   Expression{Type: TypeValue, Cmp: Ge{}, RHS: float64(80)},
		},
	}
	runScenario(t, scenarios)
}

func TestExpressionGt(t *testing.T) {
	scenarios := []Scenario{
		Scenario{
			expression: ">80%",
			expected:   Expression{Type: TypePercentage, Cmp: Gt{}, RHS: float64(80)},
		},
		Scenario{
			expression: ">80",
			expected:   Expression{Type: TypeValue, Cmp: Gt{}, RHS: float64(80)},
		},
	}
	runScenario(t, scenarios)
}

func TestExpressionLt(t *testing.T) {
	scenarios := []Scenario{
		Scenario{
			expression: "<80%",
			expected:   Expression{Type: TypePercentage, Cmp: Lt{}, RHS: float64(80)},
		},
		Scenario{
			expression: "<80",
			expected:   Expression{Type: TypeValue, Cmp: Lt{}, RHS: float64(80)},
		},
	}
	runScenario(t, scenarios)
}

func TestExpressionLe(t *testing.T) {
	scenarios := []Scenario{
		Scenario{
			expression: "<=80%",
			expected:   Expression{Type: TypePercentage, Cmp: Le{}, RHS: float64(80)},
		},
		Scenario{
			expression: "<=80",
			expected:   Expression{Type: TypeValue, Cmp: Le{}, RHS: float64(80)},
		},
	}
	runScenario(t, scenarios)
}

func TestExpressionEq(t *testing.T) {
	scenarios := []Scenario{
		Scenario{
			expression: "=80%",
			expected:   Expression{Type: TypePercentage, Cmp: Eq{}, RHS: float64(80)},
		},
		Scenario{
			expression: "=80",
			expected:   Expression{Type: TypeValue, Cmp: Eq{}, RHS: float64(80)},
		},
	}
	runScenario(t, scenarios)
}

func TestMalformedExpression(t *testing.T) {
	_, err := ParseExpression("deadbeef")
	require.Error(t, err, "expression deadbeef should be rejected as invalid")
}

func TestExpressionString(t *testing.T) {
	exp, err := ParseExpression("=100%")
	require.NoError(t, err)
	require.Equal(t, "=100%", exp.String())
}
