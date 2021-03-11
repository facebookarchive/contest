// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logrus

import (
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/pkg/xcontext/logger/internal"
	"github.com/sirupsen/logrus"
)

var (
	_ internal.LoggerCompact = Wrapper{}
)

type Fields = internal.Fields

func wrap(backend *logrus.Entry) logger.Logger {
	// Each structured logger has own types and we do intermediate methods
	// to convert one types to another. And to do not duplicate
	// Debugf/Infof/... in each such adapter we define only Logf
	// and unroll it using internal.UncompactLoggerCompact.

	// Since we do all this wrapping the caller-detection in LogRus works
	// wrong. To fix it we use a special hook `fixCallerHook`. But to do not
	// define it multiple times on each wrapping of the logger, we recheck
	// if it is already set.
	foundTheHook := false
	for _, hook := range backend.Logger.Hooks[logrus.ErrorLevel] {
		_, foundTheHook = hook.(fixCallerHook)
		if foundTheHook {
			break
		}
	}
	if !foundTheHook {
		backend.Logger.AddHook(fixCallerHook{})
	}

	// And here we do the main idea of this function (`wrap`):
	return internal.UncompactLoggerCompact{
		LoggerCompact: Wrapper{
			Backend: backend,
		},
	}
}

// Wrapper converts *logrus.Entry to internal.LoggerCompact (which could be
// easily converted to logger.Logger).
type Wrapper struct {
	Backend *logrus.Entry
}

// Logf implements internal.MinimalLoggerCompact.
func (l Wrapper) Logf(level logger.Level, format string, args ...interface{}) {
	internal.MinimalLoggerLogf(l.Backend, level, format, args...)
}

// OriginalLogger implements internal.LoggerExtensions.
func (l Wrapper) OriginalLogger() interface{} {
	return l.Backend
}

// Level implements internal.WithLevels.
func (l Wrapper) Level() logger.Level {
	return Adapter.ConvertLevel(l.Backend.Logger.Level)
}

// WithLevel implements internal.WithLevels.
func (l Wrapper) WithLevel(logLevel logger.Level) logger.Logger {
	// I haven't found an easier way to change loglevel for a child logger,
	// but not to affect the loglevel of the parent.
	newLogger := &logrus.Logger{
		Out:          l.Backend.Logger.Out,
		Hooks:        l.Backend.Logger.Hooks,
		Formatter:    l.Backend.Logger.Formatter,
		ReportCaller: l.Backend.Logger.ReportCaller,
		Level:        l.Backend.Logger.Level,
		ExitFunc:     l.Backend.Logger.ExitFunc,
	}
	newLogger.SetLevel(Adapter.Level(logLevel))
	return internal.UncompactLoggerCompact{LoggerCompact: Wrapper{
		Backend: &logrus.Entry{
			Logger:  newLogger,
			Data:    l.Backend.Data,
			Time:    l.Backend.Time,
			Context: l.Backend.Context,
		},
	}}
}

// WithField implements internal.WithFields.
func (l Wrapper) WithField(field string, value interface{}) logger.Logger {
	return wrap(l.Backend.WithField(field, value))
}

// WithFields implements internal.WithFields.
func (l Wrapper) WithFields(fields Fields) logger.Logger {
	return wrap(l.Backend.WithFields(map[string]interface{}(fields)))
}
