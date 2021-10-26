// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package internal of logger unifies different types of loggers into
// interfaces Logger. For example it allows to upgrade simple fmt.Printf
// to be a fully functional Logger. Therefore multiple wrappers are implemented
// here to provide different functions which could be missing in some loggers.
package internal

// MinimalLoggerLogf is an implementation of method Logf (of interface
// MinimalLoggerCompact) based on MinimalLogger.
//
// This function is implemented here to deduplicate code among logadapter-s.
func MinimalLoggerLogf(
	l MinimalLogger,
	level Level,
	format string,
	args ...interface{},
) {
	var logf func(string, ...interface{})
	switch level {
	case LevelDebug:
		logf = l.Debugf
	case LevelInfo:
		logf = l.Infof
	case LevelWarning:
		logf = l.Warnf
	case LevelError:
		logf = l.Errorf
	case LevelPanic:
		logf = l.Panicf
	case LevelFatal:
		logf = l.Fatalf
	default:
		return
	}
	logf(format, args...)
}
