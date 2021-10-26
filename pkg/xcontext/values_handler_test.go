// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type keyType string

func TestWithValue(t *testing.T) {
	stdCtx := context.Background()
	stdCtx = context.WithValue(stdCtx, keyType("key0"), "value0")
	stdCtx = context.WithValue(stdCtx, keyType("key1"), "value1")
	ctx := NewContext(stdCtx, "", nil, nil, nil, nil, nil)
	ctx = WithValue(ctx, keyType("key2"), "value2")
	ctx = WithValue(ctx, keyType("key3"), "value3")
	require.Equal(t, "value0", ctx.Value(keyType("key0")))
	require.Equal(t, "value1", ctx.Value(keyType("key1")))
	require.Equal(t, "value2", ctx.Value(keyType("key2")))
	require.Equal(t, "value3", ctx.Value(keyType("key3")))
	require.Equal(t, nil, ctx.Value("key3"))
	require.Equal(t, nil, ctx.Value(keyType("key4")))
}

func BenchmarkWithValue(b *testing.B) {
	key := keyType("key")
	value := "value"
	ctx := Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = WithValue(ctx, key, value)
	}
}

func BenchmarkStdWithValue(b *testing.B) {
	key := keyType("key")
	value := "value"
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = context.WithValue(ctx, key, value)
	}
}
