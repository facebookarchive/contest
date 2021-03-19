package xcontext

import (
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
)

var _ logger.MinimalLogger = &ctxValue{}

func (ctx *ctxValue) Debugf(format string, args ...interface{}) {
	ctx.Logger().Debugf(format, args...)
}

func (ctx *ctxValue) Infof(format string, args ...interface{}) {
	ctx.Logger().Infof(format, args...)
}

func (ctx *ctxValue) Warnf(format string, args ...interface{}) {
	ctx.Logger().Warnf(format, args...)
}

func (ctx *ctxValue) Errorf(format string, args ...interface{}) {
	ctx.Logger().Errorf(format, args...)
}

func (ctx *ctxValue) Panicf(format string, args ...interface{}) {
	ctx.Logger().Panicf(format, args...)
}

func (ctx *ctxValue) Fatalf(format string, args ...interface{}) {
	ctx.Logger().Panicf(format, args...)
}
