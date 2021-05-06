// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"encoding/json"
	"testing"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"

	"github.com/stretchr/testify/require"
)

var (
	ctx = logrusctx.NewContext(logger.LevelDebug)
)

// Definition of two dummy TestSteps to be used to test the PluginRegistry

// AStep implements a dummy TestStep
type AStep struct{}

// NewAStep initializes a new AStep
func NewAStep() test.TestStep {
	return &AStep{}
}

// ValidateParameters validates the parameters for the AStep
func (e AStep) ValidateParameters(ctx xcontext.Context, params test.TestStepParameters) error {
	return nil
}

// Name returns the name of the AStep
func (e AStep) Name() string {
	return "AStep"
}

// Run executes the AStep
func (e AStep) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}

func TestRegisterTestStep(t *testing.T) {
	pr := NewPluginRegistry(ctx)
	err := pr.RegisterTestStep("AStep", NewAStep, []event.Name{event.Name("AStepEventName")})
	require.NoError(t, err)
}

func TestRegisterTestStepDoesNotValidate(t *testing.T) {
	pr := NewPluginRegistry(ctx)
	err := pr.RegisterTestStep("AStep", NewAStep, []event.Name{event.Name("Event which does not validate")})
	require.Error(t, err)
}
