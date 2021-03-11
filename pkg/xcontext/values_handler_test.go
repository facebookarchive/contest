package xcontext

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithValue(t *testing.T) {
	stdCtx := context.Background()
	stdCtx = context.WithValue(stdCtx, "key0", "value0")
	stdCtx = context.WithValue(stdCtx, "key1", "value1")
	ctx := NewContext(stdCtx, "", nil, nil, nil, nil, nil)
	ctx = WithValue(ctx, "key2", "value2")
	ctx = WithValue(ctx, "key3", "value3")
	require.Equal(t, "value0", ctx.Value("key0"))
	require.Equal(t, "value1", ctx.Value("key1"))
	require.Equal(t, "value2", ctx.Value("key2"))
	require.Equal(t, "value3", ctx.Value("key3"))
	require.Equal(t, nil, ctx.Value("key4"))
}

func BenchmarkWithValue(b *testing.B) {
	key := "key"
	value := "value"
	ctx := Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = WithValue(ctx, key, value)
	}
}

func BenchmarkStdWithValue(b *testing.B) {
	key := "key"
	value := "value"
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = context.WithValue(ctx, key, value)
	}
}
