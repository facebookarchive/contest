// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"errors"
	"fmt"
	"strconv"
)

// --- "Forward declaration" from #118 to enable building DB migration logic ----

// TestStepParameters represents the parameters that a TestStep should consume
// according to the Test descriptor fetched by the TestFetcher
type StepParameters map[string][]Param

// Get returns the value of the requested parameter. A missing item is not
// distinguishable from an empty value. For this you need to use the regular map
// accessor.
func (t StepParameters) Get(k string) []Param {
	return t[k]
}

// GetOne returns the first value of the requested parameter. If the parameter
// is missing, an empty string is returned.
func (t StepParameters) GetOne(k string) *Param {
	v, ok := t[k]
	if !ok || len(v) == 0 {
		return &Param{}
	}
	return &v[0]
}

// GetInt works like GetOne, but also tries to convert the string to an int64,
// and returns an error if this fails.
func (t StepParameters) GetInt(k string) (int64, error) {
	v := t.GetOne(k)
	if v.String() == "" {
		return 0, errors.New("expected an integer string, got an empty string")
	}
	n, err := strconv.ParseInt(v.String(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot convert '%s' to int: %v", v, err)
	}
	return n, nil
}

// StepDescriptor is the definition of a test step matching a test step
// configuration.
type StepDescriptor struct {
	Name       string
	Label      string
	Parameters StepParameters
}

// StepsDescriptors bundles together Test and Cleanup descriptions
// TODO: give a less overloaded name to this structure
// (e.g. ResolvedTestDescriptor)
type StepsDescriptors struct {
	TestName string
	Test     []StepDescriptor
	Cleanup  []StepDescriptor
}

// --- End Forward declaration port from #118 ----
