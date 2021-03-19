// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package internal of logger unifies different types of loggers into
// interfaces Logger. For example it allows to upgrade simple fmt.Printf
// to be a fully functional Logger. Therefore multiple wrappers are implemented
// here to provide different functions which could be missing in some loggers.
package internal

import (
	"fmt"
)

var (
	_ Logger = ExtendWrapper{}
)

// ExtendWrapper implements Logger based on MinimalLogger.
//
// Thus if you have only a MinimalLogger, you can extend it to a Logger
// using this wrapper.
//
// It is called ExtendWrapper because it actually implements
// only LoggerExtensions.
type ExtendWrapper struct {
	MinimalLogger
	Prefix   string
	CurLevel Level
}

// OriginalLogger implements LoggerExtensions.
func (l ExtendWrapper) OriginalLogger() interface{} {
	if backend, _ := l.MinimalLogger.(interface{ OriginalLogger() interface{} }); backend != nil {
		return backend.OriginalLogger()
	}

	return l.MinimalLogger
}

// WithField implements LoggerExtensions.
func (l ExtendWrapper) WithField(key string, value interface{}) Logger {
	l.Prefix += fmt.Sprintf("%s = %v ", key, value)
	return l
}

// WithFields implements LoggerExtensions.
func (l ExtendWrapper) WithFields(fields Fields) Logger {
	for key, value := range fields {
		l.Prefix += fmt.Sprintf("%s = %v ", key, value)
	}
	return l
}

// WithLevel implements LoggerExtensions.
func (l ExtendWrapper) WithLevel(level Level) Logger {
	l.CurLevel = level
	return l
}

// Level implements LoggerExtensions.
func (l ExtendWrapper) Level() Level {
	return l.CurLevel
}

// Debugf implements MinimalLogger.
//
// It calls the backend ExtendWrapper.MinimalLogger.Debugf but also checks
// if the logging level is satisfied.
func (l ExtendWrapper) Debugf(format string, args ...interface{}) {
	if l.CurLevel < LevelDebug {
		return
	}
	l.MinimalLogger.Debugf("%s"+format, append([]interface{}{l.Prefix}, args...)...)
}

// Infof implements MinimalLogger.
//
// It calls the backend ExtendWrapper.MinimalLogger.Infof but also checks
// if the logging level is satisfied.
func (l ExtendWrapper) Infof(format string, args ...interface{}) {
	if l.CurLevel < LevelInfo {
		return
	}
	l.MinimalLogger.Infof("%s"+format, append([]interface{}{l.Prefix}, args...)...)
}

// Warnf implements MinimalLogger.
//
// It calls the backend ExtendWrapper.MinimalLogger.Warnf but also checks
// if the logging level is satisfied.
func (l ExtendWrapper) Warnf(format string, args ...interface{}) {
	if l.CurLevel < LevelWarning {
		return
	}
	l.MinimalLogger.Warnf("%s"+format, append([]interface{}{l.Prefix}, args...)...)
}

// Errorf implements MinimalLogger.
//
// It calls the backend ExtendWrapper.MinimalLogger.Errorf but also checks
// if the logging level is satisfied.
func (l ExtendWrapper) Errorf(format string, args ...interface{}) {
	if l.CurLevel < LevelError {
		return
	}
	l.MinimalLogger.Errorf("%s"+format, append([]interface{}{l.Prefix}, args...)...)
}

// Panicf implements MinimalLogger.
//
// It calls the backend ExtendWrapper.MinimalLogger.Panicf but also checks
// if the logging level is satisfied.
func (l ExtendWrapper) Panicf(format string, args ...interface{}) {
	if l.CurLevel < LevelPanic {
		return
	}
	l.MinimalLogger.Panicf("%s"+format, append([]interface{}{l.Prefix}, args...)...)
}

// Fatalf implements MinimalLogger.
//
// It calls the backend ExtendWrapper.MinimalLogger.Fatalf but also checks
// if the logging level is satisfied.
func (l ExtendWrapper) Fatalf(format string, args ...interface{}) {
	if l.CurLevel < LevelFatal {
		return
	}
	l.MinimalLogger.Fatalf("%s"+format, append([]interface{}{l.Prefix}, args...)...)
}
