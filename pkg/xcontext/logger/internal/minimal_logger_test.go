// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinimalLoggerLogf(t *testing.T) {
	m := testMinimalLogger{}

	for level := LevelFatal; level <= LevelDebug; level++ {
		MinimalLoggerLogf(&m, level, level.String(), level)
	}

	outss := map[string][]out{
		"debug":   m.debugs,
		"info":    m.infos,
		"warning": m.warns,
		"error":   m.errors,
		"panic":   m.panics,
		"fatal":   m.fatals,
	}

	for levelName, outs := range outss {
		require.Len(t, outs, 1, levelName)
		for _, out := range outs {
			require.Equal(t, levelName, out.Format)
			require.Len(t, out.Args, 1, levelName)
			require.IsType(t, Level(0), out.Args[0], levelName)
			require.IsType(t, levelName, out.Args[0].(Level).String())
		}
	}
}
