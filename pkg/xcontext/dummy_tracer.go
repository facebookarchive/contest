// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package xcontext implements a generic context with integrated logger,
// metrics, tracer and recoverer.
package xcontext

import (
	"time"
)

type dummyTracer struct{}
type dummyTracedSpan struct {
	StartTime time.Time
}

var (
	dummyTracerInstance = &dummyTracer{}
)

// StartSpan implements Tracer (see the description there).
func (tracer *dummyTracer) StartSpan(label string) TimeSpan {
	return &dummyTracedSpan{
		StartTime: time.Now(),
	}
}

// WithField implements Tracer (see the description there).
func (tracer *dummyTracer) WithField(key string, value interface{}) Tracer {
	return tracer
}

// WithFields implements Tracer (see the description there).
func (tracer *dummyTracer) WithFields(fields Fields) Tracer {
	return tracer
}

// Finish implements TimeSpan (see the description there).
func (span *dummyTracedSpan) Finish() time.Duration {
	return time.Since(span.StartTime)
}
