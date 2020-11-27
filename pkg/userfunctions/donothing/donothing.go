// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package donothing

import "errors"

var userFunctions = map[string]interface{}{
	// sample function to prove that function registration works.
	"do_nothing": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("do_nothing: no arg specified")
		}
		return a[0], nil
	},
}

// Load - Return the user-defined functions
func Load() map[string]interface{} {
	return userFunctions
}
