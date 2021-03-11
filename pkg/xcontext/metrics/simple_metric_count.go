// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package metrics

import (
	"sync"

	"go.uber.org/atomic"
)

type simpleMetricCountFamily struct {
	sync.RWMutex
	Metrics map[string]*SimpleMetricCount
}

func (family *simpleMetricCountFamily) get(tags tags) MetricCount {
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

	metric = &SimpleMetricCount{
		family: family,
	}
	family.Metrics[tagsKey] = metric

	return metric
}

// SimpleMetricCount is a naive implementation of MetricCount.
type SimpleMetricCount struct {
	family *simpleMetricCountFamily
	atomic.Uint64
}

// WithOverriddenTags implements MetricCount.
func (metric *SimpleMetricCount) WithOverriddenTags(overrideTags Fields) MetricCount {
	var tags tags
	tags.AddMultiple(overrideTags)
	return metric.family.get(tags)
}
