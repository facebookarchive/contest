// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/lib/comparison"
	"github.com/facebookincubator/contest/pkg/target"
)

// GetResult evaluates the success of list of Targets based on a comparison expression
func GetResult(targets map[*target.Target]error, ignore []*target.Target, expression string) (*comparison.Result, error) {
	var success, fail uint64
	for t, v := range targets {
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
