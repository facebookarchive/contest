// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package prometheus

import (
	"sort"
	"strings"
	"testing"

	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	metricstester "github.com/facebookincubator/contest/pkg/xcontext/metrics/test"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	for name, registerer := range map[string]prometheus.Registerer{
		"WithRegisterer":    prometheus.NewRegistry(),
		"WithoutRegisterer": nil,
	} {
		t.Run(name, func(t *testing.T) {
			metricstester.TestMetrics(t, func() metrics.Metrics {
				// Current implementation resets metrics if new label appears,
				// thus some unit-tests fails (and the should). Specifically
				// for prometheus we decided to tolerate this problem, therefore
				// adding hacks to prevent a wrong values: pre-register metrics
				// with all the labels beforehand.
				m := New(registerer)
				m.WithTags(Fields{"testField": "", "anotherField": ""}).Count("WithOverriddenTags")
				m.WithTags(Fields{"testField": "", "anotherField": ""}).Gauge("WithOverriddenTags")
				m.WithTags(Fields{"testField": "", "anotherField": ""}).IntGauge("WithOverriddenTags")

				return m
			})
		})
	}
}

func TestMetricsList(t *testing.T) {
	m := New(nil)
	m.WithTags(Fields{"testField": "", "anotherField": ""}).Count("test")
	m.WithTags(Fields{"testField": "", "anotherField": ""}).Gauge("test")
	m.WithTags(Fields{"testField": "", "anotherField": ""}).IntGauge("test")
	m.WithTags(Fields{"testField": "a", "anotherField": ""}).Count("test")
	m.WithTags(Fields{"testField": "b", "anotherField": ""}).Gauge("test")
	m.WithTags(Fields{"testField": "c", "anotherField": ""}).IntGauge("test")

	list := m.List()
	require.Len(t, list, 3)
	for _, c := range list {
		ch := make(chan prometheus.Metric, 10)
		c.Collect(ch)
		close(ch)

		count := 0
		for range ch {
			count++
		}
		require.Equal(t, 2, count)
	}
}

func TestMetricsRegistererDoubleUse(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics0 := New(registry)
	metrics1 := New(registry)

	// these test cases should panic:

	t.Run("Count", func(t *testing.T) {
		defer func() {
			require.NotNil(t, recover())
		}()

		metrics0.Count("test")
		metrics1.Count("test")
	})

	t.Run("Gauge", func(t *testing.T) {
		defer func() {
			require.NotNil(t, recover())
		}()

		metrics0.Gauge("test")
		metrics1.Gauge("test")
	})

	t.Run("IntGauge", func(t *testing.T) {
		defer func() {
			require.NotNil(t, recover())
		}()

		metrics0.IntGauge("test")
		metrics1.IntGauge("test")
	})
}

func TestMergeSortedStrings(t *testing.T) {
	slices := [][]string{
		{"a", "b", "c"},
		{"r", "a", "n", "d", "o", "m"},
		{"a", "rb", "", "it", "r", "ary"},
	}
	for idx := range slices {
		sort.Strings(slices[idx])
	}
	for _, a := range slices {
		for _, b := range slices {
			t.Run(strings.Join(a, "-")+"_"+strings.Join(b, "-"), func(t *testing.T) {
				m := map[string]struct{}{}
				for _, aItem := range a {
					m[aItem] = struct{}{}
				}
				for _, bItem := range b {
					m[bItem] = struct{}{}
				}
				expected := make([]string, 0, len(m))
				for k := range m {
					expected = append(expected, k)
				}
				sort.Strings(expected)

				require.Equal(t, expected, mergeSortedStrings(a, b...))
			})
		}
	}
}
