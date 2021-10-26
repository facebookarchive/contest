// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package prometheus

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xaionaro-go/unsafetools"
)

// prometheus does not support full unregister. Therefore we add code
// to do so.
//
// See also: https://github.com/prometheus/client_golang/issues/203
func unregister(registerer prometheus.Registerer, c prometheus.Collector) bool {
	if !registerer.Unregister(c) {
		return false
	}

	descChan := make(chan *prometheus.Desc, 10)
	go func() {
		c.Describe(descChan)
		close(descChan)
	}()

	locker := unsafetools.FieldByName(registerer, "mtx").(*sync.RWMutex)

	dimHashesByName := *unsafetools.FieldByName(registerer, "dimHashesByName").(*map[string]uint64)

	locker.Lock()
	defer locker.Unlock()

	for desc := range descChan {
		delete(dimHashesByName, *unsafetools.FieldByName(desc, "fqName").(*string))
	}

	return true
}
