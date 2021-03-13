// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package metrics

import (
	"sync"

	"go.uber.org/atomic"
)

type simpleMetricIntGaugeFamily struct {
	sync.RWMutex
	Metrics map[string]*SimpleMetricIntGauge
}

func (family *simpleMetricIntGaugeFamily) get(tags tags) MetricIntGauge {
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

	metric = &SimpleMetricIntGauge{
		Family: family,
	}
	family.Metrics[tagsKey] = metric

	return metric
}

// SimpleMetricIntGauge is a naive implementation of MetricIntGauge.
type SimpleMetricIntGauge struct {
	Family *simpleMetricIntGaugeFamily
	atomic.Int64
}

// WithOverriddenTags implements MetricIntGauge.
func (metric *SimpleMetricIntGauge) WithOverriddenTags(overrideTags Fields) MetricIntGauge {
	var tags tags
	tags.AddMultiple(overrideTags)
	return metric.Family.get(tags)
}
