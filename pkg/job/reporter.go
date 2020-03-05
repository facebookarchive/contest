// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"github.com/facebookincubator/contest/pkg/abstract"
	"github.com/facebookincubator/contest/pkg/event/testevent"
)

// ReporterFactory is a type representing a factory which builds
// a Reporter.
type ReporterFactory interface {
	abstract.Factory

	// New constructs and returns a Reporter
	New() Reporter
}

// ReporterFactories is a helper type to operate over multiple ReporterFactory-es
type ReporterFactories []ReporterFactory

// ToAbstract returns the factories as abstract.Factories
//
// Go has no contracts (yet) / traits / whatever, and Go does not allow
// to convert slice of interfaces to slice of another interfaces
// without a loop, so we have to implement this method for each
// non-abstract-factories slice
//
// TODO: try remove it when this will be implemented:
//       https://github.com/golang/proposal/blob/master/design/go2draft-contracts.md
func (reporterFactories ReporterFactories) ToAbstract() (result abstract.Factories) {
	for _, factory := range reporterFactories {
		result = append(result, factory)
	}
	return
}

// Reporter is an interface used to implement logic which calculates the result
// of a Job. The result is conveyed via a JobReport object.
type Reporter interface {
	ValidateRunParameters([]byte) (interface{}, error)
	ValidateFinalParameters([]byte) (interface{}, error)

	Name() string

	RunReport(cancel <-chan struct{}, parameters interface{}, runStatus *RunStatus, ev testevent.Fetcher) (bool, interface{}, error)
	FinalReport(cancel <-chan struct{}, parameters interface{}, runStatuses []RunStatus, ev testevent.Fetcher) (bool, interface{}, error)
}

// ReporterBundle bundles the selected Reporter together with its parameters
// based on the content of the job descriptor
type ReporterBundle struct {
	Reporter   Reporter
	Parameters interface{}
}
