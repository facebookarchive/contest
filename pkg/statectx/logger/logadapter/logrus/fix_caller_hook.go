// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logrus

import (
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	// maxCallerScanDepths defines maximum possible depth of calls of
	// logger-related functions inside the `xcontext` code.
	//
	// More this value is, slower the logger works.
	maxCallerScanDepth = 16
)

var _ logrus.Hook = fixCallerHook{}

// fixCallerHook is a hook which makes the value of entry.Caller
// more meaningful, otherwise the whole log has the same values of "file"
// and "func", like here:
//
//     "file": "github.com/facebookincubator/contest/pkg/xcontext/logger/internal/minimal_logger.go:34",
//     "func": "github.com/facebookincubator/contest/pkg/xcontext/logger/internal.MinimalLoggerLogf",
type fixCallerHook struct{}

func (fixCallerHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (fixCallerHook) Fire(entry *logrus.Entry) error {
	if entry.Caller == nil {
		// if entry.Logger.ReportCaller is false, then there's nothing to do
		return nil
	}

	if !strings.HasPrefix(entry.Caller.Function, "github.com/facebookincubator/contest/pkg/xcontext") {
		// if the value is already correct, then there's nothing to do.
		return nil
	}

	programCounters := make([]uintptr, maxCallerScanDepth)
	// we skip first two entries to skip "runtime.Callers" and "fixCallerHook.Fire".
	n := runtime.Callers(2, programCounters)
	frames := runtime.CallersFrames(programCounters[0:n])
	for {
		frame, more := frames.Next()
		if !strings.HasPrefix(frame.Function, "github.com/facebookincubator/contest/pkg/xcontext") &&
			strings.Index(strings.ToLower(frame.Function), "github.com/sirupsen/logrus") == -1 {
			entry.Caller = &frame
			return nil
		}

		if !more {
			// Unable to get out of the `xcontext` code, so using the earliest
			// function.
			//
			// If this happened consider increasing of maxCallerScanDepth
			entry.Caller = &frame
			return nil
		}
	}
}
