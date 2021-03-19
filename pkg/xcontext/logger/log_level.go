// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logger

import (
	"github.com/facebookincubator/contest/pkg/xcontext/logger/internal"
)

// Level is used to define severity of messages to be reported.
type Level = internal.Level

const (
	// LevelUndefined is the erroneous value of log-level which corresponds
	// to zero-value.
	LevelUndefined = internal.LevelUndefined

	// LevelFatal will report about Fatalf-s only.
	LevelFatal = internal.LevelFatal

	// LevelPanic will report about Panicf-s and Fatalf-s only.
	LevelPanic = internal.LevelPanic

	// LevelError will report about Errorf-s, Panicf-s, ...
	LevelError = internal.LevelError

	// LevelWarning will report about Warningf-s, Errorf-s, ...
	LevelWarning = internal.LevelWarning

	// LevelInfo will report about Infof-s, Warningf-s, ...
	LevelInfo = internal.LevelInfo

	// LevelDebug will report about Debugf-s, Infof-s, ...
	LevelDebug = internal.LevelDebug

	// EndOfLevel is just used as a limiter for `for`-s.
	EndOfLevel
)

// ParseLogLevel parses incoming string into a Level and returns
// LevelUndefined with an error if an unknown logging level was passed.
func ParseLogLevel(in string) (Level, error) {
	return internal.ParseLogLevel(in)
}
