// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/event"
)

// EventTargetIn indicates that a target has entered a Step
var EventTargetIn = event.Name("TargetIn")

// EventTargetInErr indicates that a target has encountered an error while entering a Step
var EventTargetInErr = event.Name("TargetInErr")

// EventTargetOut indicates that a target has left a Step
var EventTargetOut = event.Name("TargetOut")

// EventTargetErr indicates that a target has encountered an error in a Step
var EventTargetErr = event.Name("TargetErr")

// EventTargetAcquired indicates that a target has been acquired for a Test
var EventTargetAcquired = event.Name("TargetAcquired")

// ErrPayload represents the payload associated with a TargetErr event
type ErrPayload struct {
	Error string
}

// Target represents a target to run tests on
type Target struct {
	Name string
	ID   string
	FQDN string
}

func (t *Target) String() string {
	if t == nil {
		return "(*Target)(nil)"
	}
	return fmt.Sprintf("Target{Name: \"%s\", ID: \"%s\", FQDN: \"%s\"}", t.Name, t.ID, t.FQDN)
}

// Result models the result of a test for a specific target.
// The result might be associated with an error.
type Result struct {
	Target *Target
	Err    error
}
