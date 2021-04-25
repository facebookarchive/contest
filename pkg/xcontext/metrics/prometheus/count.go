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
	_ metrics.Count = &Count{}
)

// Count is an implementation of metrics.Count.
type Count struct {
	sync.Mutex
	*prometheus.CounterVec
	io_prometheus_client.Metric
	prometheus.Counter
}

// Add implementations metrics.Count.
func (metric *Count) Add(delta uint64) uint64 {
	metric.Counter.Add(float64(delta))
	metric.Lock()
	defer metric.Unlock()
	err := metric.Write(&metric.Metric)
	if err != nil {
		panic(err)
	}
	return uint64(*metric.Metric.Counter.Value)
}

// WithOverriddenTags implementations metrics.Count.
func (metric *Count) WithOverriddenTags(tags Fields) metrics.Count {
	return &Count{CounterVec: metric.CounterVec, Counter: metric.CounterVec.With(tagsToLabels(tags))}
}
