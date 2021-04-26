// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package tsmetrics

import (
	"testing"

	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	metricstester "github.com/facebookincubator/contest/pkg/xcontext/metrics/test"
)

func TestMetrics(t *testing.T) {
	metricstester.TestMetrics(t, func() metrics.Metrics {
		return New()
	})
}
