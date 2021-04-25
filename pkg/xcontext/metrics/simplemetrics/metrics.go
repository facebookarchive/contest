// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package simplemetrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/facebookincubator/contest/pkg/xcontext/fields"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
)

type Fields = fields.Fields
type tags = fields.PendingFields

var _ metrics.Metrics = &Metrics{}

type storage struct {
	locker           sync.RWMutex
	intGaugeFamilies map[string]*intGaugeFamily
	gaugeFamilies    map[string]*gaugeFamily
	countFamilies    map[string]*countFamily
}

// Metrics is a naive implementation of Metrics
type Metrics struct {
	*storage
	currentTags tags
}

// New returns an instance of Metrics.
func New() *Metrics {
	return &Metrics{
		storage: &storage{
			intGaugeFamilies: make(map[string]*intGaugeFamily),
			gaugeFamilies:    make(map[string]*gaugeFamily),
			countFamilies:    make(map[string]*countFamily),
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

// Count returns Count.
//
// This method implements Metrics.
func (metrics *Metrics) Count(key string) metrics.Count {
	metrics.storage.locker.RLock()
	family := metrics.countFamilies[key]
	metrics.storage.locker.RUnlock()
	if family != nil {
		return family.get(metrics.currentTags)
	}

	metrics.storage.locker.Lock()
	defer metrics.storage.locker.Unlock()
	family = metrics.countFamilies[key]
	if family != nil {
		return family.get(metrics.currentTags)
	}

	family = &countFamily{
		Metrics: make(map[string]*Count),
	}
	metrics.countFamilies[key] = family

	return family.get(metrics.currentTags)
}

// Gauge returns Gauge.
//
// This method implements Metrics.
func (metrics *Metrics) Gauge(key string) metrics.Gauge {
	metrics.storage.locker.RLock()
	family := metrics.gaugeFamilies[key]
	metrics.storage.locker.RUnlock()
	if family != nil {
		return family.get(metrics.currentTags)
	}

	metrics.storage.locker.Lock()
	defer metrics.storage.locker.Unlock()
	family = metrics.gaugeFamilies[key]
	if family != nil {
		return family.get(metrics.currentTags)
	}

	family = &gaugeFamily{
		Metrics: make(map[string]*Gauge),
	}
	metrics.gaugeFamilies[key] = family

	return family.get(metrics.currentTags)
}

// IntGauge returns IntGauge.
//
// This method implements Metrics.
func (metrics *Metrics) IntGauge(key string) metrics.IntGauge {
	metrics.storage.locker.RLock()
	family := metrics.intGaugeFamilies[key]
	metrics.storage.locker.RUnlock()
	if family != nil {
		return family.get(metrics.currentTags)
	}

	metrics.storage.locker.Lock()
	defer metrics.storage.locker.Unlock()
	family = metrics.intGaugeFamilies[key]
	if family != nil {
		return family.get(metrics.currentTags)
	}

	family = &intGaugeFamily{
		Metrics: make(map[string]*IntGauge),
	}
	metrics.intGaugeFamilies[key] = family

	return family.get(metrics.currentTags)
}

// WithTag returns a scope of Metrics with added field.
//
// This method implements Metrics.
func (metrics Metrics) WithTag(key string, value interface{}) metrics.Metrics {
	metrics.currentTags.IsReadOnly = true
	metrics.currentTags.AddOne(key, value)
	return &metrics
}

// WithTags returns a scope of Metrics with added fields.
//
// This method implements Metrics.
func (metrics Metrics) WithTags(tags Fields) metrics.Metrics {
	metrics.currentTags.IsReadOnly = true
	metrics.currentTags.AddMultiple(tags)
	return &metrics
}
