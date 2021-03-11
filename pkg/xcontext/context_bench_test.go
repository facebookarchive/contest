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
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewContext(nil, "", nil, nil, nil, nil, nil)
	}
}

func BenchmarkWithCancel(b *testing.B) {
	ctx := NewContext(nil, "", nil, nil, nil, nil, nil).WithTag("1", 2)
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
		context.WithCancel(ctx)
	}
}

func BenchmarkCtxValue_Clone(b *testing.B) {
	ctx := NewContext(nil, "", nil, nil, nil, nil, nil).WithTag("1", 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testCtx = ctx.Clone()
	}
}

func BenchmarkCtxValue_WithTag(b *testing.B) {
	ctx := NewContext(nil, "", nil, nil, nil, nil, nil).WithTag("1", 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testCtx = ctx.WithTag("2", 3)
	}
}

func BenchmarkCtxValue_WithTags(b *testing.B) {
	ctx := NewContext(nil, "", nil, nil, nil, nil, nil).WithTag("1", 2)
	tags := Fields{"2": 3}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testCtx = ctx.WithTags(tags)
	}
}
