// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/types"
)

// RunStartedPayload represents the payload carried by a failure event (e.g. JobStateFailed, JobStateCancelled, etc.)
type RunStartedPayload struct {
	RunID types.RunID
}

// EventRunStarted indicates that a run has begun
var EventRunStarted = event.Name("RunStarted")

// EventTestError indicates that a test failed.
var EventTestError = event.Name("TestError")
