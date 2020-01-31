// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.
package test

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/lib/comparison"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// Result wraps the result of a comparison, including the full expression that was evaluated
type Result struct {
	Pass bool
	Expr string
	Lhs  string
	Rhs  string
	Type comparison.Type
	Op   comparison.Operator
}

// TestResult is a type which represents the results of Targets after having gone through a test
type TestResult struct {
	results map[*target.Target]error
	JobID   types.JobID
}

// SetTarget sets the result of a target in the TestResult structure
func (r *TestResult) SetTarget(target *target.Target, err error) {
	r.results[target] = err
}

// Targets returns a map that associates each target with a possible error that
// it encountered during the run
func (r *TestResult) Targets() map[*target.Target]error {
	return r.results
}

// EvaluateSuccessRate evaluates the success of a TestResult object based on a string
// comparison expression
func (r TestResult) GetResult(expression string, ignore []*target.Target) (*comparison.Result, error) {

	var success, fail uint64
	for t, v := range r.results {
		// Evaluate whether the Target is in the ignore list
		var skip bool
		for _, ignoreTarget := range ignore {
			skip = false
			if *t == *ignoreTarget {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if v == nil {
			success++
		} else {
			fail++
		}
	}

	cmpExpr, err := comparison.ParseExpression(expression)
	if err != nil {
		return nil, fmt.Errorf("error while calculating test results: %v", err)
	}
	res, err := cmpExpr.EvaluateSuccess(success, success+fail)
	if err != nil {
		return nil, fmt.Errorf("error while calculating test results: %v", err)
	}
	return res, nil
}

// NewTestResult creates a new TestResult structure
func NewTestResult(jobID types.JobID) TestResult {
	tr := TestResult{}
	tr.results = make(map[*target.Target]error)
	tr.JobID = jobID
	return tr
}
