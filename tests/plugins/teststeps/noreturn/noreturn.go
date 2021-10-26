// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package noreturn

import (
	"encoding/json"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
var Name = "NoReturn"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

type noreturnStep struct {
}

// Name returns the name of the Step
func (ts *noreturnStep) Name() string {
	return Name
}

// Run executes a step that never returns.
func (ts *noreturnStep) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	for target := range ch.In {
		ch.Out <- test.TestStepResult{Target: target}
	}
	channel := make(chan struct{})
	<-channel
	return nil, nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *noreturnStep) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	return nil
}

// New creates a new noreturnStep which forwards targets before hanging
func New() test.TestStep {
	return &noreturnStep{}
}
