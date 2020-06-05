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

// StepParameters represents the parameters that a Step should consume
// according to the Test descriptor fetched by the TestFetcher
type StepParameters map[string][]Param

// ParameterFunc is a function type called on parameters that need further
// validation or manipulation. It is currently used by GetFunc and GetOneFunc.
type ParameterFunc func(string) string

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

// StepStatus represent a blob containing status information that the Step
// can persist into storage to support resuming the test.
type StepStatus string

// StepFactory is a type representing a function which builds a Step.
// Step factories are registered in the plugin registry.
type StepFactory func() Step

// StepLoader is a type representing a function which returns all the
// needed things to be able to load a Step.
type StepLoader func() (string, StepFactory, []event.Name)

// StepDescriptor is the definition of a test step matching a test step
// configuration.
type StepDescriptor struct {
	Name       string
	Label      string
	Parameters StepParameters
}

// StepsDescriptors bundles together Test and Cleanup descriptions
type StepsDescriptors struct {
	TestName string
	Test     []StepDescriptor
	Cleanup  []StepDescriptor
}

// StepBundle contains the instantiation of a Step object described by
// a StepDescriptor, together with its supporting information (e.g.
// input parameters).
type StepBundle struct {
	Step          Step
	StepLabel     string
	Parameters    StepParameters
	AllowedEvents map[event.Name]bool
}

// StepChannels represents the input and output  channels used by a Step
// to communicate with the TestRunner
type StepChannels struct {
	In  <-chan *target.Target
	Out chan<- *target.Target
	Err chan<- cerrors.TargetError
}

// Step is the interface that all steps need to implement to be executed
// by the TestRunner
type Step interface {
	// Name returns the name of the step
	Name() string
	// Run runs the test step. The test step is expected to be synchronous.
	Run(cancel, pause <-chan struct{}, ch StepChannels, params StepParameters, ev testevent.Emitter) error
	// CanResume signals whether a test step can be resumed.
	CanResume() bool
	// Resume is called if a test step resume is requested, and CanResume
	// returns true. If resume is not supported, this method should return
	// ErrResumeNotSupported.
	Resume(cancel, pause <-chan struct{}, ch StepChannels, params StepParameters, ev testevent.EmitterFetcher) error
	// ValidateParameters checks that the parameters are correct before passing
	// them to Run.
	ValidateParameters(params StepParameters) error
}
