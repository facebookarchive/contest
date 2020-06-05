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

// Definition of two dummy Steps to be used to test the PluginRegistry

// AStep implements a dummy Step
type AStep struct{}

// NewAStep initializes a new AStep
func NewAStep() test.Step {
	return &AStep{}
}

// ValidateParameters validates the parameters for the AStep
func (e AStep) ValidateParameters(params test.StepParameters) error {
	return nil
}

// Name returns the name of the AStep
func (e AStep) Name() string {
	return "AStep"
}

// Run executes the AStep
func (e AStep) Run(cancel, pause <-chan struct{}, ch test.StepChannels, params test.StepParameters, ev testevent.Emitter) error {
	return nil
}

// CanResume tells whether this step is able to resume.
func (e AStep) CanResume() bool {
	return false
}

// Resume tries to resume AStep
func (e AStep) Resume(cancel, pause <-chan struct{}, _ test.StepChannels, _ test.StepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: "AStep"}
}

func TestRegisterStep(t *testing.T) {
	pr := NewPluginRegistry()
	err := pr.RegisterStep("AStep", NewAStep, []event.Name{event.Name("AStepEventName")})
	require.NoError(t, err)
}

func TestRegisterStepDoesNotValidate(t *testing.T) {
	pr := NewPluginRegistry()
	err := pr.RegisterStep("AStep", NewAStep, []event.Name{event.Name("Event which does not validate")})
	require.Error(t, err)
}
