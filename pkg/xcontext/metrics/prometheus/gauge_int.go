// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package prometheus

import (
	"sync"

	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
)

var (
	_ metrics.IntGauge = &IntGauge{}
)

// IntGauge is an implementation of metrics.IntGauge.
type IntGauge struct {
	sync.Mutex
	*Metrics
	Key string
	*GaugeVec
	io_prometheus_client.Metric
	prometheus.Gauge
}

// Add implementations metrics.Count.
func (metric *IntGauge) Add(delta int64) int64 {
	metric.Gauge.Add(float64(delta))
	metric.Lock()
	defer metric.Unlock()
	err := metric.Write(&metric.Metric)
	if err != nil {
		panic(err)
	}
	return int64(*metric.Metric.Gauge.Value)
}

// WithOverriddenTags implementations metrics.Count.
func (metric *IntGauge) WithOverriddenTags(tags Fields) metrics.IntGauge {
	result, err := metric.GaugeVec.GetMetricWith(tagsToLabels(tags))
	if err == nil {
		return &IntGauge{Metrics: metric.Metrics, Key: metric.Key, GaugeVec: metric.GaugeVec, Gauge: result}
	}

	return metric.Metrics.WithTags(nil).WithTags(tags).IntGauge(metric.Key)
}
