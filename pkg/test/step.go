// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
)

// TestStepParameters represents the parameters that a TestStep should consume
// according to the Test descriptor fetched by the TestFetcher
type TestStepParameters map[string][]Param

// ParameterFunc is a function type called on parameters that need further
// validation or manipulation. It is currently used by GetFunc and GetOneFunc.
type ParameterFunc func(string) string

// Get returns the value of the requested parameter. A missing item is not
// distinguishable from an empty value. For this you need to use the regular map
// accessor.
func (t TestStepParameters) Get(k string) []Param {
	return t[k]
}

// GetOne returns the first value of the requested parameter. If the parameter
// is missing, an empty string is returned.
func (t TestStepParameters) GetOne(k string) *Param {
	v, ok := t[k]
	if !ok || len(v) == 0 {
		return NewParam("")
	}
	return &v[0]
}

// GetInt works like GetOne, but also tries to convert the string to an int64,
// and returns an error if this fails.
func (t TestStepParameters) GetInt(k string) (int64, error) {
	v := t.GetOne(k)
	if v.Raw() == "" {
		return 0, errors.New("expected an integer string, got an empty string")
	}
	n, err := strconv.ParseInt(v.Raw(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot convert '%s' to int: %v", v.Raw(), err)
	}
	return n, nil
}

// TestStepStatus represent a blob containing status information that the TestStep
// can persist into storage to support resuming the test.
type TestStepStatus string

// TestStepFactory is a type representing a function which builds a TestStep.
// TestStep factories are registered in the plugin registry.
type TestStepFactory func() TestStep

// TestStepLoader is a type representing a function which returns all the
// needed things to be able to load a TestStep.
type TestStepLoader func() (string, TestStepFactory, []event.Name)

// TestStepDescriptor is the definition of a test step matching a test step
// configuration.
type TestStepDescriptor struct {
	Name       string
	Label      string
	Parameters TestStepParameters
}

// TestStepBundle bundles the selected TestStep together with its parameters as
// specified in the Test descriptor fetched by the TestFetcher
type TestStepBundle struct {
	TestStep      TestStep
	TestStepLabel string
	TestStepIndex uint
	Parameters    TestStepParameters
	AllowedEvents map[event.Name]bool
}

// TestStepChannels represents the input and output  channels used by a TestStep
// to communicate with the TestRunner
type TestStepChannels struct {
	In  <-chan *target.Target
	Out chan<- *target.Target
	Err chan<- cerrors.TargetError
}

// TestStep is the interface that all steps need to implement to be executed
// by the TestRunner
type TestStep interface {
	// Name returns the name of the step
	Name() string
	// Run runs the test step. The test step is expected to be synchronous.
	Run(cancel, pause <-chan struct{}, ch TestStepChannels, params TestStepParameters, ev testevent.Emitter) error
	// CanResume signals whether a test step can be resumed.
	CanResume() bool
	// Resume is called if a test step resume is requested, and CanResume
	// returns true. If resume is not supported, this method should return
	// ErrResumeNotSupported.
	Resume(cancel, pause <-chan struct{}, ch TestStepChannels, params TestStepParameters, ev testevent.EmitterFetcher) error
	// ValidateParameters checks that the parameters are correct before passing
	// them to Run.
	ValidateParameters(params TestStepParameters) error
}
