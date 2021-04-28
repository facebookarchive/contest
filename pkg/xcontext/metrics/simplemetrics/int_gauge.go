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
	_ metrics.IntGauge = &IntGauge{}
)

type intGaugeFamily struct {
	sync.RWMutex
	Metrics map[string]*IntGauge
}

func (family *intGaugeFamily) get(tags tags) metrics.IntGauge {
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

	metric = &IntGauge{
		Family: family,
	}
	family.Metrics[tagsKey] = metric

	return metric
}

// IntGauge is a naive implementation of IntGauge.
type IntGauge struct {
	Family *intGaugeFamily
	atomic.Int64
}

// WithOverriddenTags implements IntGauge.
func (metric *IntGauge) WithOverriddenTags(overrideTags Fields) metrics.IntGauge {
	var tags tags
	tags.AddMultiple(overrideTags)
	return metric.Family.get(tags)
}
