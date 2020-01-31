// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"testing"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"

	"github.com/stretchr/testify/require"
)

// Definition of two dummy TestSteps to be used to test the PluginRegistry

// AStep implements a dummy TestStep
type AStep struct{}

// NewAStep initializes a new AStep
func NewAStep() test.TestStep {
	return &AStep{}
}

// ValidateParameters validates the parameters for the AStep
func (e AStep) ValidateParameters(params test.TestStepParameters) error {
	return nil
}

// Name returns the name of the AStep
func (e AStep) Name() string {
	return "AStep"
}

// Run executes the AStep
func (e AStep) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	return nil
}

// CanResume tells whether this step is able to resume.
func (e AStep) CanResume() bool {
	return false
}

// Resume tries to resume AStep
func (e AStep) Resume(cancel, pause <-chan struct{}, _ test.TestStepChannels, _ test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: "AStep"}
}

func TestRegisterTestStep(t *testing.T) {
	pr := NewPluginRegistry()
	err := pr.RegisterTestStep("AStep", NewAStep, []event.Name{event.Name("AStepEvetName")})
	require.NoError(t, err)
}

func TestRegisterTestStepDoesNotValidate(t *testing.T) {
	pr := NewPluginRegistry()
	err := pr.RegisterTestStep("AStep", NewAStep, []event.Name{event.Name("Event which does not validate")})
	require.Error(t, err)
}
