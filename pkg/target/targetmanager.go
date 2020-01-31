// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import "github.com/facebookincubator/contest/pkg/types"

// TargetManagerFactory is a type representing a function which builds
// a TargetManager.
type TargetManagerFactory func() TargetManager

// TargetManager is an interface used to acquire and release the targets to
// run tests on.
type TargetManager interface {
	ValidateAcquireParameters([]byte) (interface{}, error)
	ValidateReleaseParameters([]byte) (interface{}, error)
	Acquire(jobID types.JobID, cancel <-chan struct{}, parameters interface{}, tl Locker) ([]*Target, error)
	Release(jobID types.JobID, cancel <-chan struct{}, parameters interface{}) error
}

// TargetManagerBundle bundles the selected TargetManager together with its
// acquire and release parameters based on the content of the job descriptor
type TargetManagerBundle struct {
	TargetManager     TargetManager
	AcquireParameters interface{}
	ReleaseParameters interface{}
}
