// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package comparison

// Package comparison implements support for parsing comparision expressions of the following type:
// * Greater than, ">"
// * Greater or equal, ">="
// * Less than, "<"
// * Less or equal, "<="
// * Equal, "="
// Furthermore, the right hand side operator can be a percentage (relative comparison)
// or an absolute number (absolute comparison)
import (
	"fmt"
	"strconv"
	"strings"
)

// Type represents the type of comparison. It could be an absolute (e.g. >50) or relative comparison (e.g. >50%)
type Type uint16

// TypeNames maps a comparison.Type to its string form.
var TypeNames = map[Type]string{
	TypeValue:      "comparison_value",
	TypePercentage: "comparison_percentage",
}

// List of possible comparison types
const (
	TypeValue Type = iota
	TypePercentage
)

// String translates a Type object into its corresponding string representation
func (t Type) String() string {
	if name, ok := TypeNames[t]; ok {
		return name
	}
	return fmt.Sprintf("comparison_type_unknown(%d)", t)
}

// Operator is the string representation of any of the supported comparison operators
type Operator string

// comparison operators.
const (
	ExprGt Operator = ">"
	ExprLt Operator = "<"
	ExprGe Operator = ">="
	ExprLe Operator = "<="
	ExprEq Operator = "="
)

// Comparator is an interface that all comparators must implement
type Comparator interface {
	Compare(lhs, rhs float64) bool
	Operator() Operator
}

// Ge implements a "greater or equal" comparator
type Ge struct {
}

// Compare implement the "greater or equal" comparison
func (c Ge) Compare(lhs, rhs float64) bool {
	return lhs >= rhs
}

// Operator returns the operator applied during the comparison
func (c Ge) Operator() Operator {
	return ExprGe
}

// Gt implements a "greater than" comparator
type Gt struct {
}

// Compare implement the "greater than" comparison
func (c Gt) Compare(lhs, rhs float64) bool {
	return lhs > rhs
}

// Operator returns the operator applied during the comparison
func (c Gt) Operator() Operator {
	return ExprGt
}

// Lt implements a "less than" comparator
type Lt struct {
}

// Compare implement the "less than" comparison
func (c Lt) Compare(lhs, rhs float64) bool {
	return lhs < rhs
}

// Operator returns the operator applied during the comparison
func (c Lt) Operator() Operator {
	return ExprLt
}

// Le implements a "less or equal" comparator
type Le struct {
}

// Compare implement the "less or equal" comparison
func (c Le) Compare(lhs, rhs float64) bool {
	return lhs <= rhs
}

// Operator returns the operator applied during the comparison
func (c Le) Operator() Operator {
	return ExprLe
}

// Eq implements an "equal" comparator
type Eq struct {
}

// Compare implement the "equal" comparison
func (c Eq) Compare(lhs, rhs float64) bool {
	return lhs == rhs
}

// Operator returns the operator applied during the comparison
func (c Eq) Operator() Operator {
	return ExprEq
}

// Result wraps the result of a comparison, including the full expression that was evaluated
type Result struct {
	Pass bool
	Expr string
	Lhs  string
	Rhs  string
	Type Type
	Op   Operator
}

// Expression is a struct that represents the deserialization of a string expression.
// The deserialization includes the following:
// * The comparision type (absolute, relative)
// * The comparison function and operator
// * The right hand side operator
type Expression struct {
	expr string
	Type Type
	Cmp  Comparator
	RHS  float64
}

// Compare invokes the Compare function of the internal Comparator object
func (e Expression) EvaluateSuccess(success, total uint64) (*Result, error) {
	var lhs float64
	if e.Type == TypePercentage {
		if total == 0 {
			return nil, fmt.Errorf("cannot evaluate success rate if total is 0")
		}
		lhs = float64(success) / float64(total) * 100
	} else if e.Type == TypeValue {
		lhs = float64(success)
	} else {
		return nil, fmt.Errorf("unknown comparison type: %s", e.Type.String())
	}
	pass := e.Cmp.Compare(lhs, e.RHS)
	cmpRes := &Result{Type: e.Type, Op: e.Cmp.Operator()}
	if e.Type == TypePercentage {
		cmpRes.Lhs = fmt.Sprintf("%.2f%%", lhs)
		cmpRes.Rhs = fmt.Sprintf("%.2f%%", e.RHS)
	} else {
		cmpRes.Lhs = fmt.Sprintf("%.2f", lhs)
		cmpRes.Rhs = fmt.Sprintf("%.2f", e.RHS)
	}
	var expr string
	if pass {
		cmpRes.Pass = true
		if e.Type == TypePercentage {
			expr = fmt.Sprintf("%.2f%% %s %.2f%%", lhs, e.Cmp.Operator(), e.RHS)
		} else {
			expr = fmt.Sprintf("%.2f %s %.2f", lhs, e.Cmp.Operator(), e.RHS)
		}
	} else {
		cmpRes.Pass = false
		if e.Type == TypePercentage {
			expr = fmt.Sprintf("%.2f%% is not %s %.2f%%", lhs, string(e.Cmp.Operator()), e.RHS)
		} else {
			expr = fmt.Sprintf("%.2f is not %s %.2f", lhs, string(e.Cmp.Operator()), e.RHS)
		}
	}
	cmpRes.Expr = expr
	return cmpRes, nil
}

func (e Expression) String() string {
	return e.expr
}

// ParseExpression deserializes a string comparison expression (e.g. "">=50") into
// a Expression object. Returns an error if the expression is not supported.
func ParseExpression(expression string) (*Expression, error) {
	comparators := []Comparator{Ge{}, Le{}, Lt{}, Gt{}, Eq{}}
	for _, cmp := range comparators {
		if !strings.HasPrefix(expression, string(cmp.Operator())) {
			continue
		}
		trimmed := strings.Trim(expression, string(cmp.Operator()))

		if strings.HasSuffix(trimmed, "%") {
			_rhs, err := strconv.ParseFloat(strings.TrimRight(trimmed, "%"), 64)
			if err != nil {
				return nil, fmt.Errorf("could not extract percentage from expression: %v", err)
			}
			return &Expression{Type: TypePercentage, Cmp: cmp, RHS: _rhs, expr: expression}, nil
		}
		_rhs, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, fmt.Errorf("could not extract right hand side of the expression: %v", err)
		}
		return &Expression{Type: TypeValue, Cmp: cmp, RHS: _rhs, expr: expression}, nil
	}
	return nil, fmt.Errorf("expression %s not supported", expression)
}
