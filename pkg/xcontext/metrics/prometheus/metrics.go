// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package prometheus

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/facebookincubator/contest/pkg/xcontext/fields"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// DefaultGlobalRegister defines if it is required to register
	// metrics in the standard global prometheus register.
	DefaultGlobalRegister = true
)

var _ metrics.Metrics = &Metrics{}

type Fields = fields.Fields

type metricKey struct {
	key        string
	labelNames string
}

type storage struct {
	locker         sync.Mutex
	GlobalRegister bool
	count          map[metricKey]*prometheus.CounterVec
	gauge          map[metricKey]*prometheus.GaugeVec
	intGauge       map[metricKey]*prometheus.GaugeVec
}

// Metrics implements a wrapper of prometheus metrics to implement
// metrics.Metrics.
//
// Warning! This implementation does not remove automatically metrics, thus
// if a metric was created once, it will be kept in memory forever.
//
// If you need a version of metrics which automatically removes metrics
// non-used for a long time, then try `tsmetrics`.
type Metrics struct {
	*storage
	labels           prometheus.Labels
	labelNames       []string
	labelNamesString string
}

// New returns a new instance of Metrics
func New() *Metrics {
	m := &Metrics{
		storage: &storage{
			GlobalRegister: DefaultGlobalRegister,
			count:          map[metricKey]*prometheus.CounterVec{},
			gauge:          map[metricKey]*prometheus.GaugeVec{},
			intGauge:       map[metricKey]*prometheus.GaugeVec{},
		},
	}
	return m
}

// Count implements context.Metrics (see the description in the interface).
func (m *Metrics) Count(key string) metrics.Count {
	k := metricKey{
		key:        key,
		labelNames: m.labelNamesString,
	}
	m.locker.Lock()
	counterVer := m.count[k]
	if counterVer == nil {
		counterVer = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: key,
		}, m.labelNames)
		m.count[k] = counterVer
		if m.GlobalRegister {
			err := prometheus.DefaultRegisterer.Register(counterVer)
			if err != nil {
				panic(err)
			}
		}
	}
	m.locker.Unlock()

	return &Count{CounterVec: counterVer, Counter: counterVer.With(m.labels)}
}

// Gauge implements context.Metrics (see the description in the interface).
func (m *Metrics) Gauge(key string) metrics.Gauge {
	k := metricKey{
		key:        key,
		labelNames: m.labelNamesString,
	}
	m.locker.Lock()
	gaugeVer := m.gauge[k]
	if gaugeVer == nil {
		gaugeVer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: key,
		}, m.labelNames)
		m.gauge[k] = gaugeVer
		if m.GlobalRegister {
			err := prometheus.DefaultRegisterer.Register(gaugeVer)
			if err != nil {
				panic(err)
			}
		}
	}
	m.locker.Unlock()

	return &Gauge{GaugeVec: gaugeVer, Gauge: gaugeVer.With(m.labels)}
}

// IntGauge implements context.Metrics (see the description in the interface).
func (m *Metrics) IntGauge(key string) metrics.IntGauge {
	k := metricKey{
		key:        key,
		labelNames: m.labelNamesString,
	}
	m.locker.Lock()
	gaugeVer := m.intGauge[k]
	if gaugeVer == nil {
		gaugeVer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: key,
		}, m.labelNames)
		m.intGauge[k] = gaugeVer
		if m.GlobalRegister {
			err := prometheus.DefaultRegisterer.Register(gaugeVer)
			if err != nil {
				panic(err)
			}
		}
	}
	m.locker.Unlock()

	return &IntGauge{GaugeVec: gaugeVer, Gauge: gaugeVer.With(m.labels)}
}

// WithTag implements context.Metrics (see the description in the interface).
func (m *Metrics) WithTag(key string, value interface{}) metrics.Metrics {
	result := &Metrics{
		storage: m.storage,
	}

	result.labels = make(prometheus.Labels, len(m.labels))
	for k, v := range m.labels {
		result.labels[k] = v
	}

	result.labels[key] = TagValueToString(value)
	labelNames := make([]string, 0, len(result.labels))
	for k := range result.labels {
		labelNames = append(labelNames, k)
	}
	sort.Strings(labelNames)
	result.labelNames = labelNames
	result.labelNamesString = strings.Join(labelNames, ",")

	return result
}

// WithTags implements context.Metrics (see the description in the interface).
func (m *Metrics) WithTags(tags Fields) metrics.Metrics {
	result := &Metrics{
		storage: m.storage,
	}

	result.labels = make(prometheus.Labels, len(m.labels)+len(tags))
	for k, v := range m.labels {
		result.labels[k] = v
	}
	for k, v := range tags {
		result.labels[k] = TagValueToString(v)
	}
	labelNames := make([]string, 0, len(result.labels))
	for k := range result.labels {
		labelNames = append(labelNames, k)
	}
	sort.Strings(labelNames)
	result.labelNames = labelNames
	result.labelNamesString = strings.Join(labelNames, ",")

	return result
}

func tagsToLabels(tags Fields) prometheus.Labels {
	labels := make(prometheus.Labels, len(tags))
	for k, v := range tags {
		labels[k] = TagValueToString(v)
	}
	return labels
}

const prebakeMax = 65536

var prebackedString [prebakeMax * 2]string

func init() {
	for i := -prebakeMax; i < prebakeMax; i++ {
		prebackedString[i+prebakeMax] = strconv.FormatInt(int64(i), 10)
	}
}

func getPrebakedString(v int32) string {
	if v >= prebakeMax || -v <= -prebakeMax {
		return ""
	}
	return prebackedString[v+prebakeMax]
}

func TagValueToString(vI interface{}) string {
	switch v := vI.(type) {
	case int:
		r := getPrebakedString(int32(v))
		if len(r) != 0 {
			return r
		}
		return strconv.FormatInt(int64(v), 10)
	case uint64:
		r := getPrebakedString(int32(v))
		if len(r) != 0 {
			return r
		}
		return strconv.FormatUint(v, 10)
	case int64:
		r := getPrebakedString(int32(v))
		if len(r) != 0 {
			return r
		}
		return strconv.FormatInt(v, 10)
	case string:
		return strings.Replace(v, ",", "_", -1)
	case bool:
		switch v {
		case true:
			return "true"
		case false:
			return "false"
		}
	case []byte:
		return string(v)
	case nil:
		return "null"
	case interface{ String() string }:
		return strings.Replace(v.String(), ",", "_", -1)
	}

	return "<unknown_type>"
}
