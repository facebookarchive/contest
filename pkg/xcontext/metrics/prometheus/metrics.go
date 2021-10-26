// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package prometheus

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/facebookincubator/contest/pkg/xcontext/fields"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var _ metrics.Metrics = &Metrics{}

type Fields = fields.Fields

func mergeSortedStrings(dst []string, add ...string) []string {
	if len(add) == 0 {
		return dst
	}
	if len(dst) == 0 {
		return append([]string{}, add...)
	}

	dstLen := len(dst)

	if !sort.StringsAreSorted(dst) || !sort.StringsAreSorted(add) {
		panic(fmt.Sprintf("%v %v", sort.StringsAreSorted(dst), sort.StringsAreSorted(add)))
	}

	i, j := 0, 0
	for i < dstLen && j < len(add) {
		switch strings.Compare(dst[i], add[j]) {
		case -1:
			i++
		case 0:
			i++
			j++
		case 1:
			dst = append(dst, add[j])
			j++
		}
	}
	dst = append(dst, add[j:]...)
	sort.Strings(dst)
	return dst
}

func labelsWithPlaceholders(labels prometheus.Labels, placeholders []string) prometheus.Labels {
	if len(labels) == len(placeholders) {
		return labels
	}
	result := make(prometheus.Labels, len(placeholders))
	for k, v := range labels {
		result[k] = v
	}
	for _, s := range placeholders {
		if _, ok := result[s]; ok {
			continue
		}
		result[s] = ""
	}
	return result
}

type CounterVec struct {
	*prometheus.CounterVec
	Key            string
	PossibleLabels []string
}

func (v *CounterVec) AddPossibleLabels(newLabels []string) {
	v.PossibleLabels = mergeSortedStrings(v.PossibleLabels, newLabels...)
}

func (v *CounterVec) GetMetricWith(labels prometheus.Labels) (prometheus.Counter, error) {
	return v.CounterVec.GetMetricWith(labelsWithPlaceholders(labels, v.PossibleLabels))
}

type GaugeVec struct {
	*prometheus.GaugeVec
	Key            string
	PossibleLabels []string
}

func (v *GaugeVec) AddPossibleLabels(newLabels []string) {
	v.PossibleLabels = mergeSortedStrings(v.PossibleLabels, newLabels...)
}

func (v *GaugeVec) GetMetricWith(labels prometheus.Labels) (prometheus.Gauge, error) {
	return v.GaugeVec.GetMetricWith(labelsWithPlaceholders(labels, v.PossibleLabels))
}

type storage struct {
	locker     sync.Mutex
	registerer prometheus.Registerer
	count      map[string]*CounterVec
	gauge      map[string]*GaugeVec
	intGauge   map[string]*GaugeVec
}

// Metrics implements a wrapper of prometheus metrics to implement
// metrics.Metrics.
//
// Pretty slow and naive implementation. Could be improved by on-need basis.
// If you need a faster implementation, then try `tsmetrics`.
//
// Warning! This implementation does not remove automatically metrics, thus
// if a metric was created once, it will be kept in memory forever.
// If you need a version of metrics which automatically removes metrics
// non-used for a long time, then try `tsmetrics`.
//
// Warning! Prometheus does not support changing amount of labels for a metric,
// therefore we delete and create a metric from scratch if it is required to
// extend the set of labels. This procedure leads to a reset of the metric
// value.
// If you need a version of metrics which does not have such flaw, then
// try `tsmetrics` or `simplemetrics`.
type Metrics struct {
	*storage
	labels     prometheus.Labels
	labelNames []string
}

// New returns a new instance of Metrics
func New(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		storage: &storage{
			registerer: registerer,
			count:      map[string]*CounterVec{},
			gauge:      map[string]*GaugeVec{},
			intGauge:   map[string]*GaugeVec{},
		},
	}
	return m
}

func (m *Metrics) List() []prometheus.Collector {
	m.storage.locker.Lock()
	defer m.storage.locker.Unlock()
	result := make([]prometheus.Collector, 0, len(m.storage.count)+len(m.storage.gauge)+len(m.storage.intGauge))

	for _, count := range m.storage.count {
		result = append(result, count.CounterVec)
	}

	for _, gauge := range m.storage.gauge {
		result = append(result, gauge.GaugeVec)
	}

	for _, intGauge := range m.storage.intGauge {
		result = append(result, intGauge.GaugeVec)
	}

	return result
}

func (m *Metrics) Registerer() prometheus.Registerer {
	return m.registerer
}

func (m *Metrics) getOrCreateCountVec(key string, possibleLabelNames []string) *CounterVec {
	counterVec := m.count[key]
	if counterVec != nil {
		return counterVec
	}

	counterVec = &CounterVec{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: key + "_count",
		}, possibleLabelNames),
		Key:            key,
		PossibleLabels: possibleLabelNames,
	}

	m.count[key] = counterVec

	if m.registerer != nil {
		err := m.registerer.Register(counterVec)
		if err != nil {
			panic(fmt.Sprintf("key: '%v', err: %v", key, err))
		}
	}

	return counterVec
}

func (m *Metrics) deleteCountVec(counterVec *CounterVec) {
	if m.registerer != nil {
		if !unregister(m.registerer, counterVec.CounterVec) {
			panic(counterVec)
		}
	}
	delete(m.count, counterVec.Key)
}

// Count implements context.Metrics (see the description in the interface).
func (m *Metrics) Count(key string) metrics.Count {
	m.storage.locker.Lock()
	defer m.storage.locker.Unlock()

	counterVec := m.getOrCreateCountVec(key, m.labelNames)

	counter, err := counterVec.GetMetricWith(m.labels)
	if err != nil {
		m.deleteCountVec(counterVec)
		counterVec.AddPossibleLabels(m.labelNames)
		counterVec = m.getOrCreateCountVec(key, counterVec.PossibleLabels)
		counter, err = counterVec.GetMetricWith(m.labels)
		if err != nil {
			panic(err)
		}
	}

	return &Count{Metrics: m, Key: key, CounterVec: counterVec, Counter: counter}
}

func (m *Metrics) getOrCreateGaugeVec(key string, possibleLabelNames []string) *GaugeVec {
	gaugeVec := m.gauge[key]
	if gaugeVec != nil {
		return gaugeVec
	}

	_gaugeVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: key + "_float",
	}, possibleLabelNames)

	gaugeVec = &GaugeVec{
		GaugeVec:       _gaugeVec,
		Key:            key,
		PossibleLabels: possibleLabelNames,
	}

	m.gauge[key] = gaugeVec

	if m.registerer != nil {
		err := m.registerer.Register(gaugeVec)
		if err != nil {
			panic(fmt.Sprintf("key: '%v', err: %v", key, err))
		}
	}

	return gaugeVec
}

func (m *Metrics) deleteGaugeVec(gaugeVec *GaugeVec) {
	if m.registerer != nil {
		if !unregister(m.registerer, gaugeVec.GaugeVec) {
			panic(gaugeVec)
		}
	}
	delete(m.gauge, gaugeVec.Key)
}

// Gauge implements context.Metrics (see the description in the interface).
func (m *Metrics) Gauge(key string) metrics.Gauge {
	m.storage.locker.Lock()
	defer m.storage.locker.Unlock()

	gaugeVec := m.getOrCreateGaugeVec(key, m.labelNames)

	gauge, err := gaugeVec.GetMetricWith(m.labels)
	if err != nil {
		m.deleteGaugeVec(gaugeVec)
		gaugeVec.AddPossibleLabels(m.labelNames)
		gaugeVec = m.getOrCreateGaugeVec(key, gaugeVec.PossibleLabels)
		gauge, err = gaugeVec.GetMetricWith(m.labels)
		if err != nil {
			panic(err)
		}
	}

	return &Gauge{Metrics: m, Key: key, GaugeVec: gaugeVec, Gauge: gauge}
}

func (m *Metrics) getOrCreateIntGaugeVec(key string, possibleLabelNames []string) *GaugeVec {
	gaugeVec := m.intGauge[key]
	if gaugeVec != nil {
		return gaugeVec
	}

	_gaugeVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: key + "_int",
	}, possibleLabelNames)

	gaugeVec = &GaugeVec{
		GaugeVec:       _gaugeVec,
		Key:            key,
		PossibleLabels: possibleLabelNames,
	}

	m.intGauge[key] = gaugeVec

	if m.registerer != nil {
		err := m.registerer.Register(gaugeVec)
		if err != nil {
			panic(fmt.Sprintf("key: '%v', err: %v", key, err))
		}
	}

	return gaugeVec
}

func (m *Metrics) deleteIntGaugeVec(intGaugeVec *GaugeVec) {
	if m.registerer != nil {
		if !unregister(m.registerer, intGaugeVec) {
			panic(intGaugeVec)
		}
	}
	delete(m.intGauge, intGaugeVec.Key)
}

// IntGauge implements context.Metrics (see the description in the interface).
func (m *Metrics) IntGauge(key string) metrics.IntGauge {
	m.storage.locker.Lock()
	defer m.storage.locker.Unlock()

	gaugeVec := m.getOrCreateIntGaugeVec(key, m.labelNames)

	gauge, err := gaugeVec.GetMetricWith(m.labels)
	if err != nil {
		m.deleteIntGaugeVec(gaugeVec)
		gaugeVec.AddPossibleLabels(m.labelNames)
		gaugeVec = m.getOrCreateIntGaugeVec(key, gaugeVec.PossibleLabels)
		gauge, err = gaugeVec.GetMetricWith(m.labels)
		if err != nil {
			panic(err)
		}
	}

	return &IntGauge{Metrics: m, Key: key, GaugeVec: gaugeVec, Gauge: gauge}
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

	return result
}

// WithTags implements context.Metrics (see the description in the interface).
func (m *Metrics) WithTags(tags Fields) metrics.Metrics {
	result := &Metrics{
		storage: m.storage,
	}
	if tags == nil {
		return result
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

// TagValueToString converts any value to a string, which could be used
// as label value in prometheus.
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
