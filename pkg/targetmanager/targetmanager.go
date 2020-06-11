// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetmanager

import (
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// EventName is the name of events emitted by a target manager
// TODO: Remove it when https://github.com/facebookincubator/contest/issues/123
//       will be resolved.
const EventName = event.Name("TargetManager")

// TargetManagerFactory is a type representing a function which builds
// a TargetManager.
type TargetManagerFactory func() TargetManager

// TargetManagerLoader is a type representing a function which returns all the
// needed things to be able to load a TestStep.
type TargetManagerLoader func() (string, TargetManagerFactory)

// TargetManager is an interface used to acquire and release the targets to
// run tests on.
type TargetManager interface {
	ValidateAcquireParameters([]byte) (interface{}, error)
	ValidateReleaseParameters([]byte) (interface{}, error)
	Acquire(jobID types.JobID, cancel <-chan struct{}, parameters interface{}, eventEmitter testevent.Emitter, tl Locker) ([]*target.Target, error)
	Release(jobID types.JobID, cancel <-chan struct{}, parameters interface{}, eventEmitter testevent.Emitter) error
}

// TargetManagerBundle bundles the selected TargetManager together with its
// acquire and release parameters based on the content of the job descriptor
type TargetManagerBundle struct {
	TargetManager     TargetManager
	AcquireParameters interface{}
	ReleaseParameters interface{}
}
