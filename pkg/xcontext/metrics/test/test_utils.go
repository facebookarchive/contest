// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package metricstester

import (
	"testing"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	"github.com/stretchr/testify/require"
)

type Fields = xcontext.Fields

// testMetric tests metric and returns the resulting value of the metric
func testMetric(t *testing.T, metrics metrics.Metrics, key string, tags Fields, overrideTags Fields, metricType string, expectedValue float64) float64 {
	switch metricType {
	case "Count":
		metric := metrics.WithTags(tags).Count(key)
		if overrideTags != nil {
			metric = metric.WithOverriddenTags(overrideTags)
		}
		require.Equal(t, uint64(expectedValue+1), metric.Add(1))
		metric = metrics.WithTags(tags).Count(key)
		if overrideTags != nil {
			metric = metric.WithOverriddenTags(overrideTags)
		}
		require.Equal(t, uint64(expectedValue+2), metric.Add(1))
		scope := metrics
		if overrideTags != nil {
			scope = scope.WithTags(nil)
			tags = overrideTags
		}
		for k, v := range tags {
			scope = scope.WithTag(k, v)
		}
		metric = scope.Count(key)
		require.Equal(t, uint64(expectedValue+3), metric.Add(1))
		return float64(metric.Add(0))
	case "Gauge":
		metric := metrics.WithTags(tags).Gauge(key)
		if overrideTags != nil {
			metric = metric.WithOverriddenTags(overrideTags)
		}
		require.Equal(t, expectedValue-1.5, metric.Add(-1.5))
		metric = metrics.WithTags(tags).Gauge(key)
		if overrideTags != nil {
			metric = metric.WithOverriddenTags(overrideTags)
		}
		require.Equal(t, expectedValue-0.5, metric.Add(1))
		scope := metrics
		if overrideTags != nil {
			scope = scope.WithTags(nil)
			tags = overrideTags
		}
		for k, v := range tags {
			scope = scope.WithTag(k, v)
		}
		metric = scope.Gauge(key)
		require.Equal(t, expectedValue+0.5, metric.Add(1))
		require.Equal(t, expectedValue, metric.Add(-0.5))
		return metric.Add(0)
	case "IntGauge":
		metric := metrics.WithTags(tags).IntGauge(key)
		if overrideTags != nil {
			metric = metric.WithOverriddenTags(overrideTags)
		}
		require.Equal(t, int64(-1), metric.Add(-1))
		metric = metrics.WithTags(tags).IntGauge(key)
		if overrideTags != nil {
			metric = metric.WithOverriddenTags(overrideTags)
		}
		require.Equal(t, int64(0), metric.Add(1))
		scope := metrics
		if overrideTags != nil {
			scope = scope.WithTags(nil)
			tags = overrideTags
		}
		for k, v := range tags {
			scope = scope.WithTag(k, v)
		}
		metric = scope.IntGauge(key)
		require.Equal(t, int64(2), metric.Add(2))
		require.Equal(t, int64(0), metric.Add(-2))
		return float64(metric.Add(0))
	}

	panic(metricType)
}

func TestMetrics(t *testing.T, metricsFactory func() metrics.Metrics) {
	m := metricsFactory()

	for _, metricType := range []string{"Count", "Gauge", "IntGauge"} {
		t.Run(metricType, func(t *testing.T) {
			t.Run("hello_world", func(t *testing.T) {
				testMetric(t, m, "hello_world", nil, nil, metricType, 0)
				testMetric(t, m, "hello_world", Fields{
					"testField": "testValue",
				}, nil, metricType, 0)
				testMetric(t, m, "hello_world", Fields{
					"testField": "anotherValue",
				}, nil, metricType, 0)
				testMetric(t, m, "hello_world", Fields{
					"anotherField": "testValue",
				}, nil, metricType, 0)
				testMetric(t, m, "hello_world", Fields{
					"testField":    "testValue",
					"anotherField": "anotherValue",
				}, nil, metricType, 0)
			})
			t.Run("WithOverriddenTags", func(t *testing.T) {
				key := "WithOverriddenTags"
				tags := Fields{
					"testField": "testValue",
				}
				prevResult := testMetric(t, m, key, tags, nil, metricType, 0)

				wrongTags := Fields{
					"testField": "anotherValue",
				}
				prevResult = testMetric(t, m, key, wrongTags, tags, metricType, prevResult)

				wrongTags = Fields{
					"anotherField": "anotherValue",
				}
				testMetric(t, m, key, wrongTags, tags, metricType, prevResult)
			})
		})
	}
}
