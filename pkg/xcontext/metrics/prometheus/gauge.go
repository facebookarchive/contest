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
	_ metrics.Gauge = &Gauge{}
)

// Gauge is an implementation of metrics.Gauge.
type Gauge struct {
	sync.Mutex
	*prometheus.GaugeVec
	io_prometheus_client.Metric
	prometheus.Gauge
}

// Add implementations metrics.Count.
func (metric *Gauge) Add(delta float64) float64 {
	metric.Gauge.Add(delta)
	metric.Lock()
	defer metric.Unlock()
	err := metric.Write(&metric.Metric)
	if err != nil {
		panic(err)
	}
	return *metric.Metric.Gauge.Value
}

// WithOverriddenTags implementations metrics.Count.
func (metric *Gauge) WithOverriddenTags(tags Fields) metrics.Gauge {
	return &Gauge{GaugeVec: metric.GaugeVec, Gauge: metric.GaugeVec.With(tagsToLabels(tags))}
}
