// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package simplemetrics

import (
	"sync"

	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	"go.uber.org/atomic"
)

var (
	_ metrics.Gauge = &Gauge{}
)

type gaugeFamily struct {
	sync.RWMutex
	Metrics map[string]*Gauge
}

func (family *gaugeFamily) get(tags tags) metrics.Gauge {
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

	metric = &Gauge{
		Family: family,
	}
	family.Metrics[tagsKey] = metric

	return metric
}

// Gauge is a naive implementation of Gauge.
type Gauge struct {
	Family *gaugeFamily
	atomic.Float64
}

// WithOverriddenTags implements Gauge.
func (metric *Gauge) WithOverriddenTags(overrideTags Fields) metrics.Gauge {
	var tags tags
	tags.AddMultiple(overrideTags)
	return metric.Family.get(tags)
}
