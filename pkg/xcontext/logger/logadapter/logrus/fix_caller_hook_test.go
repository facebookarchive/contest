// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logrus

import (
	"runtime"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func dummyFunc(fn func()) {
	fn()
}

func TestFixCallerHook(t *testing.T) {
	programCounters := make([]uintptr, 1)
	n := runtime.Callers(1, programCounters)
	require.Equal(t, 1, n)
	frames := runtime.CallersFrames(programCounters)
	frame, more := frames.Next()
	require.False(t, more)
	entry := &logrus.Entry{Caller: &frame}
	dummyFunc(func() {
		err := fixCallerHook{}.Fire(entry)
		require.NoError(t, err)
	})

	// testing.tRunner is the function which calls TestFixCallerHook
	require.Equal(t, "testing.tRunner", entry.Caller.Func.Name())
}
