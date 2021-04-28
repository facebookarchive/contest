// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package zap

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/facebookincubator/contest/pkg/xcontext/fields"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/pkg/xcontext/logger/internal"
)

var (
	_ internal.LoggerCompact = Wrapper{}
)

type Fields = fields.Fields

func wrap(backend *zap.SugaredLogger) logger.Logger {
	// Each structured logger has own types and we do intermediate methods
	// to convert one types to another. And to do not duplicate
	// Debugf/Infof/... in each such adapter we define only Logf
	// and unroll it using internal.UncompactLoggerCompact.
	return internal.UncompactLoggerCompact{
		LoggerCompact: Wrapper{
			Backend: backend,
		},
	}
}

// Wrapper converts *zap.SugaredLogger to internal.LoggerCompact (which could be
// easily converted to logger.Logger).
type Wrapper struct {
	Backend *zap.SugaredLogger
}

// Logf implements internal.MinimalLoggerCompact.
func (l Wrapper) Logf(level logger.Level, format string, args ...interface{}) {
	// Passing through "format" for better automatic error categorization
	// by Sentry-like services. This way we can detect which exactly
	// Errorf line was used even when lines being shifted up and down.
	internal.MinimalLoggerLogf(l.Backend.Named(format), level, format, args...)
}

// OriginalLogger implements internal.LoggerExtensions.
func (l Wrapper) OriginalLogger() interface{} {
	return l.Backend
}

// Level implements internal.WithLevels.
func (l Wrapper) Level() logger.Level {
	enabledFn := l.Backend.Desugar().Core().Enabled
	for _, level := range []zapcore.Level{
		zap.DebugLevel,
		zap.InfoLevel,
		zap.WarnLevel,
		zap.ErrorLevel,
		zap.DPanicLevel,
		zap.PanicLevel,
		zap.FatalLevel,
	} {
		if enabledFn(level) {
			return Adapter.ConvertLevel(level)
		}
	}

	return logger.LevelUndefined
}

// WithLevel implements internal.WithLevels.
func (l Wrapper) WithLevel(logLevel logger.Level) logger.Logger {
	return wrap(
		l.Backend.Desugar().
			WithOptions(
				zap.IncreaseLevel(Adapter.Level(logLevel)),
			).Sugar(),
	)
}

// WithField implements internal.WithFields.
func (l Wrapper) WithField(field string, value interface{}) logger.Logger {
	return wrap(l.Backend.With(field, value))
}

// WithFields implements internal.WithFields.
func (l Wrapper) WithFields(fields Fields) logger.Logger {
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return wrap(l.Backend.With(args...))
}
