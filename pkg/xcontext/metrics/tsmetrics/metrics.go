// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package tsmetrics

import (
	"github.com/facebookincubator/contest/pkg/xcontext/fields"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	tsmetrics "github.com/xaionaro-go/metrics"
)

var _ metrics.Metrics = &Metrics{}

type Fields = fields.Fields

// Metrics implements a wrapper of github.com/xaionaro-go/metrics to implement
// metrics.Metrics.
type Metrics struct {
	Registry *tsmetrics.Registry
	Tags     *tsmetrics.FastTags
}

// New returns a new instance of Metrics
func New() *Metrics {
	m := &Metrics{
		Registry: tsmetrics.New(),
		Tags:     tsmetrics.NewFastTags().(*tsmetrics.FastTags),
	}
	m.Registry.SetDefaultGCEnabled(true)
	m.Registry.SetDefaultIsRan(true)
	m.Registry.SetSender(nil)
	return m
}

// Count implements context.Metrics (see the description in the interface).
func (m *Metrics) Count(key string) metrics.Count {
	return Count{Metrics: m, MetricCount: m.Registry.Count(key, m.Tags)}
}

// Gauge implements context.Metrics (see the description in the interface).
func (m *Metrics) Gauge(key string) metrics.Gauge {
	return Gauge{Metrics: m, MetricGaugeFloat64: m.Registry.GaugeFloat64(key, m.Tags)}
}

// IntGauge implements context.Metrics (see the description in the interface).
func (m *Metrics) IntGauge(key string) metrics.IntGauge {
	return IntGauge{Metrics: m, MetricGaugeInt64: m.Registry.GaugeInt64(key, m.Tags)}
}

// WithTag implements context.Metrics (see the description in the interface).
func (m *Metrics) WithTag(key string, value interface{}) metrics.Metrics {
	newTags := tsmetrics.NewFastTags().(*tsmetrics.FastTags)
	newTags.Slice = make([]*tsmetrics.FastTag, len(m.Tags.Slice))
	copy(newTags.Slice, m.Tags.Slice)
	newTags.Set(key, value)
	return &Metrics{
		Registry: m.Registry,
		Tags:     newTags,
	}
}

// WithTags implements context.Metrics (see the description in the interface).
func (m *Metrics) WithTags(tags Fields) metrics.Metrics {
	newTags := tsmetrics.NewFastTags().(*tsmetrics.FastTags)
	newTags.Slice = make([]*tsmetrics.FastTag, len(m.Tags.Slice))
	copy(newTags.Slice, m.Tags.Slice)
	for k, v := range tags {
		newTags.Set(k, v)
	}
	return &Metrics{
		Registry: m.Registry,
		Tags:     newTags,
	}
}
