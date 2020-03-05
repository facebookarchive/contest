// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"github.com/facebookincubator/contest/pkg/abstract"
	"github.com/facebookincubator/contest/pkg/types"
)

// TargetManagerFactory is a type representing a factory which builds
// a TargetManager.
type TargetManagerFactory interface {
	abstract.Factory

	// New constructs and returns a TargetManager
	New() TargetManager
}

// TargetManagerFactories is a helper type to operate over multiple TargetManagerFactory-es
type TargetManagerFactories []TargetManagerFactory

// ToAbstract returns the factories as abstract.Factories
//
// Go has no contracts (yet) / traits / whatever, and Go does not allow
// to convert slice of interfaces to slice of another interfaces
// without a loop, so we have to implement this method for each
// non-abstract-factories slice
//
// TODO: try remove it when this will be implemented:
//       https://github.com/golang/proposal/blob/master/design/go2draft-contracts.md
func (targetManagerFactories TargetManagerFactories) ToAbstract() (result abstract.Factories) {
	for _, factory := range targetManagerFactories {
		result = append(result, factory)
	}
	return
}

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
