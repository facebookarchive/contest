// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package zapctx

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	zapadapter "github.com/facebookincubator/contest/pkg/xcontext/logger/logadapter/zap"
	prometheusadapter "github.com/facebookincubator/contest/pkg/xcontext/metrics/prometheus"
)

// NewContext is a simple-to-use function to create a context.Context
// using zap as the logger.
//
// See also: https://github.com/uber-go/zap
func NewContext(logLevel logger.Level, opts ...bundles.Option) xcontext.Context {
	cfg := bundles.GetConfig(opts...)
	loggerCfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapadapter.Adapter.Level(logLevel)),
		Development:      true,
		Encoding:         "console",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
	switch cfg.Format {
	case bundles.LogFormatPlainText, bundles.LogFormatPlainTextCompact:
		loggerCfg.Encoding = "console"
	case bundles.LogFormatJSON:
		loggerCfg.Encoding = "json"
	}
	if cfg.TimestampFormat != "" {
		loggerCfg.EncoderConfig.EncodeTime = timeEncoder(cfg.TimestampFormat)
	}
	// TODO: cfg.VerboseCaller is currently ignored, fix it.
	var zapOpts []zap.Option
	stdCtx := context.Background()
	loggerRaw, err := loggerCfg.Build(zapOpts...)
	if err != nil {
		panic(err)
	}
	loggerInstance := logger.ConvertLogger(loggerRaw.Sugar())
	ctx := xcontext.NewContext(
		stdCtx, "",
		loggerInstance, prometheusadapter.New(prometheus.DefaultRegisterer), cfg.Tracer,
		nil, nil)
	return ctx
}

func timeEncoder(layout string) func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	return func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		type appendTimeEncoder interface {
			AppendTimeLayout(time.Time, string)
		}

		if enc, ok := enc.(appendTimeEncoder); ok {
			enc.AppendTimeLayout(t, layout)
			return
		}

		enc.AppendString(t.Format(layout))
	}
}
