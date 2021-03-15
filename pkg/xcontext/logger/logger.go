// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logger

import (
	"github.com/facebookincubator/contest/pkg/xcontext/logger/internal"
)

// Logger is the interface of a logger provided by an extended context.
//
// This is the complete logger with all the required methods/features.
type Logger = internal.Logger

// MinimalLogger is the interfaces of a logger without contextual methods.
type MinimalLogger = internal.MinimalLogger

// ConvertLogger converts arbitrary logger to Logger. If it was unable to
// then it returns a nil.
//
// To enable logadapter-s for this converter add imports, for example:
//
//     import _ "github.com/facebookincubator/contest/pkg/xcontext/logger/logadapter/logrus"
//     import _ "github.com/facebookincubator/contest/pkg/xcontext/logger/logadapter/zap"
func ConvertLogger(logger interface{}) Logger {
	if l, ok := logger.(Logger); ok {
		return l
	}

	for _, adapter := range adapters {
		if convertedLogger := adapter.Convert(logger); convertedLogger != nil {
			return convertedLogger
		}
	}

	switch logger := logger.(type) {
	case internal.MinimalLogger:
		return internal.WrapMinimalLogger(logger)
	case internal.MinimalLoggerCompact:
		return internal.WrapMinimalLoggerCompact(logger)
	case func(format string, args ...interface{}):
		return internal.WrapPrintf(logger)
	case func(format string, args ...interface{}) (int, error):
		return internal.WrapPrintf(func(format string, args ...interface{}) {
			_, _ = logger(format, args...)
		})
	}

	return nil
}

// Adapter defines an adapter to convert a logger to Logger.
type Adapter interface {
	Convert(backend interface{}) Logger
}

var (
	adapters []Adapter
)

// RegisterAdapter registers an Adapter to be used by ConvertLogger.
func RegisterAdapter(adapter Adapter) {
	adapters = append(adapters, adapter)
}

// Dummy returns a dummy logger which implements Logger, but does nothing.
func Dummy() Logger {
	return ConvertLogger(internal.DummyLogger{})
}
