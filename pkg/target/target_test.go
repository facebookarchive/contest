// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNilTarget(t *testing.T) {
	var recoverResult interface{}
	func() {
		defer func() {
			recoverResult = recover()
		}()
		_ = (*Target)(nil).String()
	}()
	require.Nil(t, recoverResult)
}
