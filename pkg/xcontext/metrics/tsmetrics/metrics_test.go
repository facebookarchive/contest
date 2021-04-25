// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package tsmetrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	metrics := New()
	metrics.WithTags(Fields{
		"testField": "testValue",
	}).Count("test").Add(1)
	require.Equal(t, uint64(1), metrics.WithTags(Fields{
		"testField": "testValue",
	}).Count("test").Add(0))
}
