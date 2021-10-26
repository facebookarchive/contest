// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package noop

import (
	"encoding/json"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "Noop"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

type noop struct {
}

// Name returns the name of the Step
func (ts *noop) Name() string {
	return Name
}

// Run executes a step that does nothing and returns targets with success.
func (ts *noop) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	return teststeps.ForEachTarget(Name, ctx, ch, func(ctx xcontext.Context, t *target.Target) error {
		return nil
	})
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *noop) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	return nil
}

// New creates a new noop step
func New() test.TestStep {
	return &noop{}
}
