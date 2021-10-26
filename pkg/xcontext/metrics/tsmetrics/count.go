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
	_ metrics.Count = &Count{}
)

// Count is an implementation of metrics.Count.
type Count struct {
	*Metrics
	*tsmetrics.MetricCount
}

// Add implementations metrics.Count.
func (metric Count) Add(delta uint64) uint64 {
	return uint64(metric.MetricCount.Add(int64(delta)))
}

// WithOverriddenTags implementations metrics.Count.
func (metric Count) WithOverriddenTags(tags Fields) metrics.Count {
	key := metric.MetricCount.GetName()
	return Count{
		Metrics:     metric.Metrics,
		MetricCount: metric.Metrics.Registry.Count(key, tsmetrics.Tags(tags)),
	}
}
