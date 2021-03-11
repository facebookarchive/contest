// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapPrintf(t *testing.T) {
	var outs []out

	l := WrapPrintf(func(format string, args ...interface{}) {
		outs = append(outs, out{
			Format: format,
			Args:   args,
		})
	})

	l.Debugf("should not be added, since default logging level is WARN", ":(")

	l.WithLevel(LevelDebug).Debugf("test", ":)")
	require.Len(t, outs, 1)
	require.Equal(t, outs[0].Format, "[debug] %stest")
	require.Len(t, outs[0].Args, 2)
	require.Equal(t, outs[0].Args[0], "") // empty prefix added by ExtendWrapper
	require.Equal(t, outs[0].Args[1], ":)")
}
