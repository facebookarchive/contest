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
	_ metrics.Count = &Count{}
)

type countFamily struct {
	sync.RWMutex
	Metrics map[string]*Count
}

func (family *countFamily) get(tags tags) metrics.Count {
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

	metric = &Count{
		family: family,
	}
	family.Metrics[tagsKey] = metric

	return metric
}

// Count is a naive implementation of Count.
type Count struct {
	family *countFamily
	atomic.Uint64
}

// WithOverriddenTags implements Count.
func (metric *Count) WithOverriddenTags(overrideTags Fields) metrics.Count {
	var tags tags
	tags.AddMultiple(overrideTags)
	return metric.family.get(tags)
}
