// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/test"
)

// ReporterFactory is a type representing a function which builds a Reporter object
type ReporterFactory func() Reporter

// ReporterLoader is a type representing a function which returns all the
// needed things to be able to load a Reporter object
type ReporterLoader func() (string, ReporterFactory)

// Reporter is an interface used to implement logic which calculates the result
// of a Job. The result is conveyed via a JobReport object.
type Reporter interface {
	ValidateParameters([]byte) (interface{}, error)
	Report(cancel <-chan struct{}, parameters interface{}, result *test.TestResult, ev testevent.Fetcher) (bool, interface{}, error)
}

// ReporterBundle bundles the selected Reporter together with its parameters
// based on the content of the job descriptor
type ReporterBundle struct {
	Reporter   Reporter
	Parameters interface{}
}
