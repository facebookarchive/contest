// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package internal

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	_ MinimalLoggerCompact = &testLoggerCompact{}
)

type testLoggerCompact struct {
	outs [LevelDebug + 1][]out
}

func (l *testLoggerCompact) Logf(level Level, format string, args ...interface{}) {
	l.outs[level] = append(l.outs[level], out{
		Format: format,
		Args:   args,
	})
}

func TestUncompactMinimalLoggerCompact(t *testing.T) {
	m := testLoggerCompact{}
	w := UncompactMinimalLoggerCompact{
		MinimalLoggerCompact: &m,
	}

	w.Debugf("debug", "DEBUG")
	w.Infof("info", "INFO")
	w.Warnf("warning", "WARNING")
	w.Errorf("error", "ERROR")
	w.Panicf("panic", "PANIC")
	w.Fatalf("fatal", "FATAL")

	for level := LevelFatal; level <= LevelDebug; level++ {
		assert.Len(t, m.outs[level], 1, level)
		assert.Equal(t, level.String(), m.outs[level][0].Format, level)
		assert.Len(t, m.outs[level][0].Args, 1, level)
		assert.Equal(t, strings.ToUpper(level.String()), m.outs[level][0].Args[0], level)
	}

	assert.Equal(t, &m, w.OriginalLogger())
}
