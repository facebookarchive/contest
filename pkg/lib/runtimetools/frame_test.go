// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runtimetools_test

import (
	"strings"
	"testing"

	. "github.com/facebookincubator/contest/pkg/lib/runtimetools"
	"github.com/stretchr/testify/assert"
)

func testFrame(t *testing.T) {
	assert.True(t, strings.HasSuffix(Frame(0).Function, `.testFrame`), Frame(0))
	assert.True(t, strings.HasSuffix(Frame(1).Function, `.TestFrame`), Frame(1))
	assert.Empty(t, Frame(65536))
}

func TestFrame(t *testing.T) {
	testFrame(t)
}
