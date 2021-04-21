// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"time"

	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

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
	Acquire(ctx xcontext.Context, jobID types.JobID, jobTargetManagerAcquireTimeout time.Duration, parameters interface{}, tl Locker) ([]*Target, error)
	Release(ctx xcontext.Context, jobID types.JobID, targets []*Target, parameters interface{}) error
}

// TargetManagerBundle bundles the selected TargetManager together with its
// acquire and release parameters based on the content of the job descriptor
type TargetManagerBundle struct {
	TargetManager     TargetManager
	AcquireParameters interface{}
	ReleaseParameters interface{}
}
