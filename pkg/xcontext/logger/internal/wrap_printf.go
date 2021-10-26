// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package internal of logger unifies different types of loggers into
// interfaces Logger. For example it allows to upgrade simple fmt.Printf
// to be a fully functional Logger. Therefore multiple wrappers are implemented
// here to provide different functions which could be missing in some loggers.
package internal

// WrapPrintf wraps a Printf-like function to provide a Logger.
func WrapPrintf(fn func(format string, args ...interface{})) Logger {
	return ExtendWrapper{
		MinimalLogger: UncompactMinimalLoggerCompact{
			MinimalLoggerCompact: printfWrapper{
				Func: fn,
			},
		},
		Prefix:   "",
		CurLevel: LevelWarning,
	}
}

var (
	_ MinimalLoggerCompact = printfWrapper{}
)

// printfWrapper converts a Printf-like function to a MinimalLoggerCompact.
type printfWrapper struct {
	Func func(format string, args ...interface{})
}

// Logf implements MinimalLoggerCompact.
func (l printfWrapper) Logf(level Level, format string, args ...interface{}) {
	l.Func("["+level.String()+"] "+format, args...)
}

// OriginalLogger implements LoggerExtensions.
func (l printfWrapper) OriginalLogger() interface{} {
	return l.Func
}
