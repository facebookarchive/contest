// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"testing"
)

func requireErrorType(t *testing.T, err, errType error) {
	// An old Go does not support verb `%w`, so there no sense
	// to check the error type using `errors.As`... Doing nothing here:
}
