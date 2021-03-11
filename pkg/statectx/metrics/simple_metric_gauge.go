// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package metrics

import (
	"sync"

	"go.uber.org/atomic"
)

type simpleMetricGaugeFamily struct {
	sync.RWMutex
	Metrics map[string]*SimpleMetricGauge
}

func (family *simpleMetricGaugeFamily) get(tags tags) MetricGauge {
	tagsKey := tagsToString(tags)

	family.RLock()
	metric := family.Metrics[tagsKey]
	family.RUnlock()
	if metric != nil {
		return metric
	}

	family.Lock()
	defer family.Unlock()
	metric = family.Metrics[tagsKey]
	if metric != nil {
		return metric
	}

	metric = &SimpleMetricGauge{
		Family: family,
	}
	family.Metrics[tagsKey] = metric

	return metric
}

// SimpleMetricGauge is a naive implementation of MetricGauge.
type SimpleMetricGauge struct {
	Family *simpleMetricGaugeFamily
	atomic.Float64
}

// OverrideTags implements MetricGauge.
func (metric *SimpleMetricGauge) WithOverriddenTags(overrideTags Fields) MetricGauge {
	var tags tags
	tags.AddMultiple(overrideTags)
	return metric.Family.get(tags)
}
