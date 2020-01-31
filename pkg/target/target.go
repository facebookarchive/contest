// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/event"
)

// TargetIn indicates that a target has entered a TestStep
var EventTargetIn = event.Name("TargetIn")

// TargetInErr indicates that a target has encountered an error while entering a TestStep
var EventTargetInErr = event.Name("TargetInErr")

// TargetOut indicates that a target has left a TestStep
var EventTargetOut = event.Name("TargetOut")

// TargetErr indicates that a target has encountered an error in a TestStep
var EventTargetErr = event.Name("TargetErr")

// Target represents a target to run tests on
type Target struct {
	Name string
	ID   string
	FQDN string
}

func (t *Target) String() string {
	return fmt.Sprintf("Target{Name: \"%s\", ID: \"%s\", FQDN: \"%s\"}", t.Name, t.ID, t.FQDN)
}
