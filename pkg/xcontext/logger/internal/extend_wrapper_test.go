// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	_ MinimalLogger = &testMinimalLogger{}
)

type out struct {
	Format string
	Args   []interface{}
}

type testMinimalLogger struct {
	debugs []out
	infos  []out
	warns  []out
	errors []out
	panics []out
	fatals []out
}

func (l *testMinimalLogger) Debugf(format string, args ...interface{}) {
	l.debugs = append(l.debugs, out{
		Format: format,
		Args:   args,
	})
}

func (l *testMinimalLogger) Infof(format string, args ...interface{}) {
	l.infos = append(l.infos, out{
		Format: format,
		Args:   args,
	})
}

func (l *testMinimalLogger) Warnf(format string, args ...interface{}) {
	l.warns = append(l.warns, out{
		Format: format,
		Args:   args,
	})
}

func (l *testMinimalLogger) Errorf(format string, args ...interface{}) {
	l.errors = append(l.errors, out{
		Format: format,
		Args:   args,
	})
}

func (l *testMinimalLogger) Panicf(format string, args ...interface{}) {
	l.panics = append(l.panics, out{
		Format: format,
		Args:   args,
	})
}

func (l *testMinimalLogger) Fatalf(format string, args ...interface{}) {
	l.fatals = append(l.fatals, out{
		Format: format,
		Args:   args,
	})
}

func TestExtendWrapper(t *testing.T) {
	m := testMinimalLogger{}
	w := ExtendWrapper{
		MinimalLogger: &m,
		Prefix:        "prefix",
	}

	// Dataset is to verify if the correct level is used and if loglevels conditions
	// are complied.
	w.WithLevel(LevelDebug).Debugf("debug", "ok")
	w.WithLevel(LevelInfo).Debugf("debug", "not ok")
	w.WithLevel(LevelInfo).Infof("info", "ok")
	w.WithLevel(LevelWarning).Infof("info", "not ok")
	w.WithLevel(LevelWarning).Warnf("warn", "ok")
	w.WithLevel(LevelError).Warnf("warn", "not ok")
	w.WithLevel(LevelError).Errorf("error", "ok")
	w.WithLevel(LevelPanic).Errorf("error", "not ok")
	w.WithLevel(LevelPanic).Panicf("panic", "ok")
	w.WithLevel(LevelFatal).Panicf("panic", "not ok")
	w.WithLevel(LevelFatal).Fatalf("fatal", "ok")

	outss := map[string][]out{
		"debug": m.debugs,
		"info":  m.infos,
		"warn":  m.warns,
		"error": m.errors,
		"panic": m.panics,
		"fatal": m.fatals,
	}

	for levelName, outs := range outss {
		require.Len(t, outs, 1, levelName)
		for _, out := range outs {
			require.Equal(t, "%s"+levelName, out.Format)
			require.Equal(t, w.Prefix, out.Args[0])
			require.Equal(t, "ok", out.Args[1])
		}
	}

	require.Equal(t, &m, w.OriginalLogger())
}
