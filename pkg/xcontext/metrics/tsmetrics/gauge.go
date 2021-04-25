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
	_ metrics.Gauge = &Gauge{}
)

// Gauge is an implementation of metrics.Gauge.
type Gauge struct {
	*Metrics
	*tsmetrics.MetricGaugeFloat64
}

// Add implementations metrics.Count.
func (metric Gauge) Add(delta float64) float64 {
	return metric.MetricGaugeFloat64.Add(delta)
}

// WithOverriddenTags implementations metrics.Count.
func (metric Gauge) WithOverriddenTags(tags Fields) metrics.Gauge {
	key := metric.MetricGaugeFloat64.GetName()
	return Gauge{
		Metrics:            metric.Metrics,
		MetricGaugeFloat64: metric.Metrics.Registry.GaugeFloat64(key, tsmetrics.Tags(tags)),
	}
}
