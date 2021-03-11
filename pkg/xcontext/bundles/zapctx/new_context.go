// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package zapctx

import (
	"context"

	"go.uber.org/zap"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	zapadapter "github.com/facebookincubator/contest/pkg/xcontext/logger/logadapter/zap"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
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
	case bundles.LogFormatPlainText:
		loggerCfg.Encoding = "console"
	case bundles.LogFormatJSON:
		loggerCfg.Encoding = "json"
	}
	var zapOpts []zap.Option
	stdCtx := context.Background()
	loggerRaw, err := loggerCfg.Build(zapOpts...)
	if err != nil {
		panic(err)
	}
	loggerInstance := logger.ConvertLogger(loggerRaw.Sugar())
	ctx := xcontext.NewContext(
		stdCtx, "",
		loggerInstance, metrics.NewSimpleMetrics(), cfg.Tracer,
		nil, nil)
	return ctx
}
