// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package metrics

import (
	"github.com/facebookincubator/contest/pkg/xcontext/internal/fields"
)

type Fields = fields.Fields
type tags = fields.PendingFields

// Metrics is a handler of metrics (like Prometheus, ODS)
type Metrics interface {
	// Gauge returns the float64 gauge metric with key "key".
	Gauge(key string) MetricGauge

	// IntGauge returns the int64 gauge metric with key "key".
	IntGauge(key string) MetricIntGauge

	// Count returns the counter metric with key "key".
	Count(key string) MetricCount

	// WithTag returns scoped Metrics with an added tag to be reported with the metrics.
	//
	// In terms of Prometheus the "tags" are called "labels".
	WithTag(key string, value interface{}) Metrics

	// WithTags returns scoped Metrics with added tags to be reported with the metrics.
	//
	// In terms of Prometheus the "tags" are called "labels".
	WithTags(fields Fields) Metrics
}

// MetricGauge is a float64 gauge metric.
//
// See also https://prometheus.io/docs/concepts/metric_types/
type MetricGauge interface {
	// Add adds value "v" to the metric and returns the result.
	Add(v float64) float64

	// WithOverriddenTags returns scoped MetricGauge with replaced tags to be reported with the metrics.
	//
	// In terms of Prometheus the "tags" are called "labels".
	WithOverriddenTags(fields.Fields) MetricGauge
}

// MetricIntGauge is a int64 gauge metric.
//
// See also https://prometheus.io/docs/concepts/metric_types/
type MetricIntGauge interface {
	// Add adds value "v" to the metric and returns the result.
	Add(v int64) int64

	// WithOverriddenTags returns scoped MetricIntGauge with replaced tags to be reported with the metrics.
	//
	// In terms of Prometheus the "tags" are called "labels".
	WithOverriddenTags(Fields) MetricIntGauge
}

// MetricCount is a counter metric.
//
// See also https://prometheus.io/docs/concepts/metric_types/
type MetricCount interface {
	// Add adds value "v" to the metric and returns the result.
	Add(v uint64) uint64

	// WithOverriddenTags returns scoped MetricCount with replaced tags to be reported with the metrics.
	//
	// In terms of Prometheus the "tags" are called "labels".
	WithOverriddenTags(Fields) MetricCount
}
