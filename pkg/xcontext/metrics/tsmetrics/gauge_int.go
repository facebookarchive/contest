// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package tsmetrics

import (
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	tsmetrics "github.com/xaionaro-go/metrics"
)

var (
	_ metrics.IntGauge = &IntGauge{}
)

// IntGauge is an implementation of metrics.IntGauge.
type IntGauge struct {
	*Metrics
	*tsmetrics.MetricGaugeInt64
}

// Add implementations metrics.Count.
func (metric IntGauge) Add(delta int64) int64 {
	return metric.MetricGaugeInt64.Add(delta)
}

// WithOverriddenTags implementations metrics.Count.
func (metric IntGauge) WithOverriddenTags(tags Fields) metrics.IntGauge {
	key := metric.MetricGaugeInt64.GetName()
	return IntGauge{
		Metrics:          metric.Metrics,
		MetricGaugeInt64: metric.Metrics.Registry.GaugeInt64(key, tsmetrics.Tags(tags)),
	}
}
