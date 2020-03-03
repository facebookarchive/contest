// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTargets_Find(t *testing.T) {
	targets := Targets{{ID: ""}, {ID: "1"}, {ID: "c"}, {ID: "m"}, {ID: "a"}, {ID: "b"}, {ID: "z"}}
	targets.Sort()
	aTarget := targets.Find("a")
	if !assert.NotNil(t, aTarget) {
		return
	}
	assert.Equal(t, "a", aTarget.ID)
}
