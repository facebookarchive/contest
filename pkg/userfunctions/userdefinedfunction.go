// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package userdefinedfunction

// UserDefinedFunction Interface - Every UDF needs to implement at least these
// functions in order to get hook up within ConTest.
type UserDefinedFunction interface {
	// Name returns the name of the step
	Name() string
	// Run runs the test step. The test step is expected to be synchronous.
	Load() (error, map[string]interface{})
}
