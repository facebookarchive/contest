// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package zap

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/facebookincubator/contest/pkg/xcontext/logger"
)

func init() {
	logger.RegisterAdapter(Adapter)
}

type adapter struct{}

var (
	_ logger.Adapter = Adapter
)

// Adapter is implementation of logger.Adapter for zap.
var Adapter = (*adapter)(nil)

// Convert implements logger.Adapter.
func (_ *adapter) Convert(backend interface{}) logger.Logger {
	switch backend := backend.(type) {
	case *zap.Logger:
		return wrap(backend.Sugar())
	case *zap.SugaredLogger:
		return wrap(backend)
	}
	return nil
}

// Level converts our logging level to zap logging level.
func (_ *adapter) Level(logLevel logger.Level) zapcore.Level {
	switch logLevel {
	case logger.LevelDebug:
		return zapcore.DebugLevel
	case logger.LevelInfo:
		return zapcore.InfoLevel
	case logger.LevelWarning:
		return zapcore.WarnLevel
	case logger.LevelError:
		return zapcore.ErrorLevel
	case logger.LevelPanic:
		return zapcore.DPanicLevel
	case logger.LevelFatal:
		return zapcore.FatalLevel
	default:
		panic(fmt.Sprintf("should never happened: %v", logLevel))
	}
}

// Level converts zap logging level to our logging level.
func (_ *adapter) ConvertLevel(level zapcore.Level) logger.Level {
	switch level {
	case zapcore.DebugLevel:
		return logger.LevelDebug
	case zapcore.InfoLevel:
		return logger.LevelInfo
	case zapcore.WarnLevel:
		return logger.LevelWarning
	case zapcore.ErrorLevel:
		return logger.LevelError
	case zapcore.PanicLevel, zapcore.DPanicLevel:
		return logger.LevelPanic
	case zapcore.FatalLevel:
		return logger.LevelFatal

	}
	return logger.LevelUndefined
}
