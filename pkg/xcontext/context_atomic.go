// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"sync/atomic"
	"unsafe"
)

func (ctx *ctxValue) loadLoggerInstance() Logger {
	return *(*Logger)(atomic.LoadPointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.loggerInstance))))
}

func (ctx *ctxValue) storeLoggerInstance(newLogger Logger) {
	atomic.StorePointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.loggerInstance)), unsafe.Pointer(&newLogger))
}

func (ctx *ctxValue) loadMetricsInstance() Metrics {
	return *(*Metrics)(atomic.LoadPointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.metricsInstance))))
}

func (ctx *ctxValue) storeMetricsInstance(newMetrics Metrics) {
	atomic.StorePointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.metricsInstance)), unsafe.Pointer(&newMetrics))
}

func (ctx *ctxValue) loadTracerInstance() Tracer {
	return *(*Tracer)(atomic.LoadPointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.tracerInstance))))
}

func (ctx *ctxValue) storeTracerInstance(newTracer Tracer) {
	atomic.StorePointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.tracerInstance)), unsafe.Pointer(&newTracer))
}
