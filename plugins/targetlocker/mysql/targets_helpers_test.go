// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package mysql implements target.Locker using a MySQL table as the backend storage.
// The same table could be safely used by multiple instances of ConTest.
package mysql

import (
	"testing"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/stretchr/testify/require"
)

func TestTargetsFind(t *testing.T) {
	targets := []*target.Target{{ID: ""}, {ID: "1"}, {ID: "c"}, {ID: "m"}, {ID: "a"}, {ID: "b"}, {ID: "z"}}
	targetsSort(targets)
	aTarget := targetsFind(targets, "a")
	require.NotNil(t, aTarget)
	require.Equal(t, "a", aTarget.ID)
}
