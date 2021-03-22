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
	"github.com/facebookincubator/contest/pkg/xcontext/fields"
)

// Fields is a multiple of fields which are attached to logger/tracer messages.
type Fields = fields.Fields

// Logger is the interface of a logger provided by an extended context.
//
// This is the complete logger with all the required methods/features.
type Logger interface {
	MinimalLogger
	LoggerExtensions
}

// LoggerCompact is a kind of logger with one method Logf(level, ...) instead
// of separate methods Debugf(...), Infof(...), Warnf(...) and so on.
type LoggerCompact interface {
	MinimalLoggerCompact
	LoggerExtensions
}

// LoggerExtensions includes all the features required for a Logger over
// MinimalLogger.
type LoggerExtensions interface {
	// OriginalLogger returns the original logger without wrappers added
	// by this package.
	OriginalLogger() interface{}

	WithLevels
	WithFields
}

// WithLevels requires methods of a logger with support of logging levels.
type WithLevels interface {
	// Level returns the current logging level.
	Level() Level

	// WithLevel returns a logger with logger level set to the passed argument.
	WithLevel(Level) Logger
}

// WithFields requires methods of a logger with support of structured logging.
type WithFields interface {
	// WithField returns a logger with added field (used for structured logging).
	WithField(key string, value interface{}) Logger

	// WithFields returns a logger with added fields (used for structured logging).
	WithFields(fields Fields) Logger
}

// MinimalLogger is a simple logger, for example logrus.Entry or zap.SugaredLogger
// could be candidates of an implementation of this interfaces.
//
// Note: logrus.Entry and zap.SugaredLogger supports structured logging and
// they have With* methods, but the API is different, so we do not require
// that methods here. We add support of builtin structured logging and
// logging levels through logadapter-s, see for example
// logadapter/logrus.Adapter.Convert.
type MinimalLogger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Panicf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

type MinimalLoggerCompact interface {
	Logf(logLevel Level, format string, args ...interface{})
}
