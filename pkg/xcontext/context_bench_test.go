// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package xcontext implements a generic context with integrated logger,
// metrics, tracer and recoverer.
package xcontext

import (
	"context"
	"testing"
)

var (
	testCtx context.Context
)

func BenchmarkNewContext(b *testing.B) {
	bgCtx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewContext(bgCtx, "", nil, nil, nil, nil, nil)
	}
}

func BenchmarkWithCancel(b *testing.B) {
	ctx := NewContext(context.Background(), "", nil, nil, nil, nil, nil).WithTag("1", 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		WithCancel(ctx)
	}
}

func BenchmarkStdWithCancel(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, c := context.WithCancel(ctx)
		// fighting govet:
		_ = c
	}
}

func BenchmarkCtxValue_Clone(b *testing.B) {
	ctx := NewContext(context.Background(), "", nil, nil, nil, nil, nil).WithTag("1", 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testCtx = ctx.Clone()
	}
}

func BenchmarkCtxValue_WithTag(b *testing.B) {
	ctx := NewContext(context.Background(), "", nil, nil, nil, nil, nil).WithTag("1", 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testCtx = ctx.WithTag("2", 3)
	}
}

func BenchmarkCtxValue_WithTags(b *testing.B) {
	ctx := NewContext(context.Background(), "", nil, nil, nil, nil, nil).WithTag("1", 2)
	tags := Fields{"2": 3}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testCtx = ctx.WithTags(tags)
	}
}
