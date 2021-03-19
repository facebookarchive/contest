// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var _ Metrics = &SimpleMetrics{}

type simpleMetricsStorage struct {
	locker           sync.RWMutex
	intGaugeFamilies map[string]*simpleMetricIntGaugeFamily
	gaugeFamilies    map[string]*simpleMetricGaugeFamily
	countFamilies    map[string]*simpleMetricCountFamily
}

// SimpleMetrics is a naive implementation of Metrics
type SimpleMetrics struct {
	*simpleMetricsStorage
	currentTags tags
}

// NewSimpleMetrics returns an instance of SimpleMetrics.
func NewSimpleMetrics() *SimpleMetrics {
	return &SimpleMetrics{
		simpleMetricsStorage: &simpleMetricsStorage{
			intGaugeFamilies: make(map[string]*simpleMetricIntGaugeFamily),
			gaugeFamilies:    make(map[string]*simpleMetricGaugeFamily),
			countFamilies:    make(map[string]*simpleMetricCountFamily),
		},
	}
}

func tagsToString(tags tags) string {
	m := tags.Compile()
	tagStrings := make([]string, 0, len(m))
	for key, value := range m {
		tagStrings = append(tagStrings, strings.ReplaceAll(fmt.Sprintf("%s=%v", key, value), ",", "_"))
	}
	sort.Slice(tagStrings, func(i, j int) bool {
		return tagStrings[i] < tagStrings[j]
	})
	return strings.Join(tagStrings, ",")
}

// Count returns MetricCount.
//
// This method implements Metrics.
func (metrics *SimpleMetrics) Count(key string) MetricCount {
	metrics.simpleMetricsStorage.locker.RLock()
	family := metrics.countFamilies[key]
	metrics.simpleMetricsStorage.locker.RUnlock()
	if family != nil {
		return family.get(metrics.currentTags)
	}

	metrics.simpleMetricsStorage.locker.Lock()
	defer metrics.simpleMetricsStorage.locker.Unlock()
	family = metrics.countFamilies[key]
	if family != nil {
		return family.get(metrics.currentTags)
	}

	family = &simpleMetricCountFamily{
		Metrics: make(map[string]*SimpleMetricCount),
	}
	metrics.countFamilies[key] = family

	return family.get(metrics.currentTags)
}

// Gauge returns MetricGauge.
//
// This method implements Metrics.
func (metrics *SimpleMetrics) Gauge(key string) MetricGauge {
	metrics.simpleMetricsStorage.locker.RLock()
	family := metrics.gaugeFamilies[key]
	metrics.simpleMetricsStorage.locker.RUnlock()
	if family != nil {
		return family.get(metrics.currentTags)
	}

	metrics.simpleMetricsStorage.locker.Lock()
	defer metrics.simpleMetricsStorage.locker.Unlock()
	family = metrics.gaugeFamilies[key]
	if family != nil {
		return family.get(metrics.currentTags)
	}

	family = &simpleMetricGaugeFamily{
		Metrics: make(map[string]*SimpleMetricGauge),
	}
	metrics.gaugeFamilies[key] = family

	return family.get(metrics.currentTags)
}

// IntGauge returns MetricIntGauge.
//
// This method implements Metrics.
func (metrics *SimpleMetrics) IntGauge(key string) MetricIntGauge {
	metrics.simpleMetricsStorage.locker.RLock()
	family := metrics.intGaugeFamilies[key]
	metrics.simpleMetricsStorage.locker.RUnlock()
	if family != nil {
		return family.get(metrics.currentTags)
	}

	metrics.simpleMetricsStorage.locker.Lock()
	defer metrics.simpleMetricsStorage.locker.Unlock()
	family = metrics.intGaugeFamilies[key]
	if family != nil {
		return family.get(metrics.currentTags)
	}

	family = &simpleMetricIntGaugeFamily{
		Metrics: make(map[string]*SimpleMetricIntGauge),
	}
	metrics.intGaugeFamilies[key] = family

	return family.get(metrics.currentTags)
}

// WithTag returns a scope of Metrics with added field.
//
// This method implements Metrics.
func (metrics SimpleMetrics) WithTag(key string, value interface{}) Metrics {
	metrics.currentTags.IsReadOnly = true
	metrics.currentTags.AddOne(key, value)
	return &metrics
}

// WithTags returns a scope of Metrics with added fields.
//
// This method implements Metrics.
func (metrics SimpleMetrics) WithTags(fields Fields) Metrics {
	metrics.currentTags.IsReadOnly = true
	metrics.currentTags.AddMultiple(fields)
	return &metrics
}
