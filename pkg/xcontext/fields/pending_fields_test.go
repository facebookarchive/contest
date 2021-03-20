// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package fields

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	testPendingFields = PendingFields{
		Slice: Slice{
			{
				parentPendingFields: Slice{{
					oneFieldKey:   "1",
					oneFieldValue: 2,
				}},
			},
			{
				oneFieldKey:   "3",
				oneFieldValue: 4,
			},
			{
				multipleFields: map[string]interface{}{
					"5": 6,
				},
			},
		},
	}
)

var v PendingFields

func BenchmarkPendingFields_Clone(b *testing.B) {
	// Last benchmark: BenchmarkPendingFields_Clone-8  967913209  1.12 ns/op  0 B/op  0 allocs/op
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v = testPendingFields.Clone()
	}
}

func BenchmarkPendingFields_CloneAndAddOne(b *testing.B) {
	// Last benchmark: BenchmarkPendingFields_CloneAndAddOne-8  17389500  74.5 ns/op  128 B/op  1 allocs
	//
	// This allocation and CPU consumption could be reduced via sync.Pool,
	// but for now it is a premature optimization (there was no request on
	// such performance).
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := testPendingFields.Clone()
		f.AddOne("1", "2")
	}
}

func BenchmarkPendingFields_CloneAndAddOneAsMultiple(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := testPendingFields.Clone()
		f.AddMultiple(Fields{"1": "2"})
	}
}

func BenchmarkPendingFields_Compile(b *testing.B) {
	// Last benchmark: BenchmarkPendingFields_Compile-8  4787438  251 ns/op  336 B/op  2 allocs
	//
	// Could be performed the same optimization as for BenchmarkPendingFields_CloneAndAddOne
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testPendingFields.Compile()
	}
}

func TestPendingFields_Compile(t *testing.T) {
	require.Equal(t, Fields{
		"1": 2,
		"3": 4,
		"5": 6,
	}, testPendingFields.Compile())
}

func TestPendingFields_Clone(t *testing.T) {
	t.Run("safeScoping", func(t *testing.T) {
		v0 := testPendingFields.Clone()
		v1 := v0.Clone()
		v0.AddOne("k", "v")
		require.Equal(t, testPendingFields.Compile(), v1.Compile())
		require.NotEqual(t, v0.Compile(), v1.Compile())
	})
}
