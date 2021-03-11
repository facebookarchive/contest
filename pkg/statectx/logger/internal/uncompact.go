// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package internal of logger unifies different types of loggers into
// interfaces Logger. For example it allows to upgrade simple fmt.Printf
// to be a fully functional Logger. Therefore multiple wrappers are implemented
// here to provide different functions which could be missing in some loggers.
package internal

var (
	_ MinimalLogger = UncompactMinimalLoggerCompact{}
)

// UncompactMinimalLoggerCompact converts MinimalLoggerCompact to MinimalLogger.
type UncompactMinimalLoggerCompact struct {
	MinimalLoggerCompact
}

// Debugf implements MinimalLogger.
func (l UncompactMinimalLoggerCompact) Debugf(format string, args ...interface{}) {
	l.Logf(LevelDebug, format, args...)
}

// Infof implements MinimalLogger.
func (l UncompactMinimalLoggerCompact) Infof(format string, args ...interface{}) {
	l.Logf(LevelInfo, format, args...)
}

// Warnf implements MinimalLogger.
func (l UncompactMinimalLoggerCompact) Warnf(format string, args ...interface{}) {
	l.Logf(LevelWarning, format, args...)
}

// Errorf implements MinimalLogger.
func (l UncompactMinimalLoggerCompact) Errorf(format string, args ...interface{}) {
	l.Logf(LevelError, format, args...)
}

// Panicf implements MinimalLogger.
func (l UncompactMinimalLoggerCompact) Panicf(format string, args ...interface{}) {
	l.Logf(LevelPanic, format, args...)
}

// Fatalf implements MinimalLogger.
func (l UncompactMinimalLoggerCompact) Fatalf(format string, args ...interface{}) {
	l.Logf(LevelFatal, format, args...)
}

// OriginalLogger implements LoggerExtensions.
func (l UncompactMinimalLoggerCompact) OriginalLogger() interface{} {
	if backend, _ := l.MinimalLoggerCompact.(interface{ OriginalLogger() interface{} }); backend != nil {
		return backend.OriginalLogger()
	}

	return l.MinimalLoggerCompact
}

var (
	_ Logger = UncompactLoggerCompact{}
)

// UncompactLoggerCompact converts LoggerCompact to Logger.
type UncompactLoggerCompact struct {
	LoggerCompact
}

// Debugf implements MinimalLogger.
func (l UncompactLoggerCompact) Debugf(format string, args ...interface{}) {
	l.Logf(LevelDebug, format, args...)
}

// Infof implements MinimalLogger.
func (l UncompactLoggerCompact) Infof(format string, args ...interface{}) {
	l.Logf(LevelInfo, format, args...)
}

// Warnf implements MinimalLogger.
func (l UncompactLoggerCompact) Warnf(format string, args ...interface{}) {
	l.Logf(LevelWarning, format, args...)
}

// Errorf implements MinimalLogger.
func (l UncompactLoggerCompact) Errorf(format string, args ...interface{}) {
	l.Logf(LevelError, format, args...)
}

// Panicf implements MinimalLogger.
func (l UncompactLoggerCompact) Panicf(format string, args ...interface{}) {
	l.Logf(LevelPanic, format, args...)
}

// Fatalf implements MinimalLogger.
func (l UncompactLoggerCompact) Fatalf(format string, args ...interface{}) {
	l.Logf(LevelFatal, format, args...)
}

// OriginalLogger implements LoggerExtensions.
func (l UncompactLoggerCompact) OriginalLogger() interface{} {
	if backend, _ := l.LoggerCompact.(interface{ OriginalLogger() interface{} }); backend != nil {
		return backend.OriginalLogger()
	}

	return l.LoggerCompact
}
