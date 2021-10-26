// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package hanging

import (
	"encoding/json"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
var Name = "Hanging"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

type hanging struct {
}

// Name returns the name of the Step
func (ts *hanging) Name() string {
	return Name
}

// Run executes a step that does not process any targets and never returns.
func (ts *hanging) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	channel := make(chan struct{})
	<-channel
	return nil, nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *hanging) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	return nil
}

// New creates a new hanging step
func New() test.TestStep {
	return &hanging{}
}
