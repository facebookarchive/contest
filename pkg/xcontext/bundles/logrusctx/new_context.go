// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logrusctx

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/sirupsen/logrus"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	logrusadapter "github.com/facebookincubator/contest/pkg/xcontext/logger/logadapter/logrus"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
)

// NewContext is a simple-to-use function to create a context.Context
// using logrus as the logger.
//
// Note: the upstream "logrus" is in maintenance-mode.
// A quote from the README.md:
//
// > Logrus is in maintenance-mode. We will not be introducing new features.
// > It's simply too hard to do in a way that won't break many people's
// > projects, which is the last thing you want from your Logging library (again...).
// >
// > This does not mean Logrus is dead. Logrus will continue to be maintained
// > for security, (backwards compatible) bug fixes, and performance (where we
// > are limited by the interface).
// >
// > I believe Logrus' biggest contribution is to have played a part in today's
// > widespread use of structured logging in Golang. There doesn't seem to be
// > a reason to do a major, breaking iteration into Logrus V2, since the
// > fantastic Go community has built those independently. Many fantastic
// > alternatives have sprung up. Logrus would look like those, had it been
// > re-designed with what we know about structured logging in Go today.
// > Check out, for example, Zerolog, Zap, and Apex.
func NewContext(logLevel logger.Level, opts ...bundles.Option) xcontext.Context {
	cfg := bundles.GetConfig(opts...)
	loggerRaw := logrus.New()
	loggerRaw.SetLevel(logrusadapter.Adapter.Level(logLevel))
	loggerRaw.ReportCaller = cfg.LoggerReportCaller
	entry := logrus.NewEntry(loggerRaw)

	var callerFormatter func(frame *runtime.Frame) (function string, file string)
	if !cfg.VerboseCaller {
		callerFormatter = func(frame *runtime.Frame) (function string, file string) {
			if frame == nil {
				return
			}
			file = fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
			return
		}
	}
	switch cfg.Format {
	case bundles.LogFormatJSON:
		entry.Logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat:  cfg.TimestampFormat,
			CallerPrettyfier: callerFormatter,
		})
	default:
		entry.Logger.SetFormatter(&logrus.TextFormatter{
			TimestampFormat:  cfg.TimestampFormat,
			FullTimestamp:    cfg.TimestampFormat != "",
			CallerPrettyfier: callerFormatter,
		})
	}
	ctx := xcontext.NewContext(
		context.Background(), "",
		logrusadapter.Adapter.Convert(entry), metrics.NewSimpleMetrics(), cfg.Tracer,
		nil, nil)
	return ctx
}
