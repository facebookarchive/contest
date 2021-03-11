// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logrus

import (
	"github.com/sirupsen/logrus"

	"github.com/facebookincubator/contest/pkg/xcontext/logger"
)

func init() {
	logger.RegisterAdapter(Adapter)
}

type adapter struct{}

var (
	_ logger.Adapter = Adapter
)

// Adapter is implementation of logger.Adapter for logrus.
var Adapter = (*adapter)(nil)

// Convert implements logger.Adapter.
func (_ *adapter) Convert(backend interface{}) logger.Logger {
	switch backend := backend.(type) {
	case *logrus.Entry:
		return wrap(backend)
	case *logrus.Logger:
		return wrap(logrus.NewEntry(backend))
	}
	return nil
}

// Level converts our logging level to logrus logging level.
func (_ *adapter) Level(level logger.Level) logrus.Level {
	switch level {
	case logger.LevelDebug:
		return logrus.DebugLevel
	case logger.LevelInfo:
		return logrus.InfoLevel
	case logger.LevelWarning:
		return logrus.WarnLevel
	case logger.LevelError:
		return logrus.ErrorLevel
	case logger.LevelPanic:
		return logrus.PanicLevel
	case logger.LevelFatal:
		return logrus.FatalLevel
	case logger.LevelUndefined:
	}

	return logrus.PanicLevel
}

// Level converts logrus logging level to our logging level.
func (_ *adapter) ConvertLevel(level logrus.Level) logger.Level {
	switch level {
	case logrus.DebugLevel:
		return logger.LevelDebug
	case logrus.InfoLevel:
		return logger.LevelInfo
	case logrus.WarnLevel:
		return logger.LevelWarning
	case logrus.ErrorLevel:
		return logger.LevelError
	case logrus.PanicLevel:
		return logger.LevelPanic
	case logrus.FatalLevel:
		return logger.LevelFatal

	}
	return logger.LevelUndefined
}
