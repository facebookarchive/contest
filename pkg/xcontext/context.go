// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package xcontext implements a generic context with integrated logger,
// metrics, tracer and recoverer.

package xcontext

import (
	"context"
	"os"
	"os/user"
	"runtime"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/xcontext/buildinfo"
	"github.com/facebookincubator/contest/pkg/xcontext/fields"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/pkg/xcontext/metrics"
	"github.com/google/uuid"
)

var (
	// DefaultLogTraceID defines if traceID should be logged by default.
	//
	// If it is disabled, then logging of traceID for a specific
	// context could be enforced this way:
	//     ctx = ctx.WithField("traceID", ctx.TraceID())
	DefaultLogTraceID = false

	// DefaultLogHostname defines if hostname should be logged by default.
	DefaultLogHostname = false

	// DefaultLogUsername defines if hostname should be logged by default.
	DefaultLogUsername = false
)

// Fields is a multiple of fields which are attached to logger/tracer messages.
type Fields = fields.Fields

// TraceID is a passthrough ID used to track a sequence of events across
// multiple processes/services. It is supposed to pass it with Thrift-requests
// through a HTTP-header "X-Trace-Id".
type TraceID string

// String implements fmt.Stringer.
func (traceID TraceID) String() string {
	return string(traceID)
}

// NewTraceID returns a new random unique TraceID.
func NewTraceID() TraceID {
	return TraceID(uuid.New().String())
}

// Logger is an abstract logger used by a Context.
type Logger = logger.Logger

// Context is a generic extension over context.Context with provides also:
// * Logger which allows to send messages to a log.
// * Metrics which allows to update metrics.
// * Tracer which allows to log time spans (to profile delays of the application).
// * TraceID to track across multiple processes.
type Context interface {
	context.Context
	logger.MinimalLogger

	// Clone just returns a copy of the Context safe to be modified.
	Clone() Context

	// TraceID returns the TraceID attached to the Context
	TraceID() TraceID

	// WithTraceID returns a clone of the Context, but with passed TraceID.
	WithTraceID(TraceID) Context

	// Logger returns the Logger handler attached to the Context
	Logger() Logger

	// WithLogger returns a clone of the Context, but with passed Logger handler.
	WithLogger(logger Logger) Context

	// Metrics returns the Metrics handler attached to the Context.
	Metrics() Metrics

	// WithMetrics returns a clone of the Context, but with passed Metrics handler.
	WithMetrics(Metrics) Context

	// Tracer returns the Tracer handler attached to the Context.
	Tracer() Tracer

	// WithTracer returns a clone of the Context, but with passed Tracer handler.
	WithTracer(Tracer) Context

	// WithTag returns a clone of the context, but with added tag with key
	// "key" and value "value".
	//
	// Note about Tag vs Field: Tag is supposed to be used for limited amount
	// of values, while Field is supposed to be used for arbitrary values.
	// Basically currentTags are used for everything (Logger, Metrics and Tracer),
	// while Fields are used only for Logger and Tracer.
	// We cannot use arbitrary values for Metrics because it will create
	// "infinite" amount of metrics.
	WithTag(key string, value interface{}) Context

	// WithTags returns a clone of the context, but with added tags with
	// key and values according to map "Fields".
	//
	// See also WithTag.
	WithTags(Fields) Context

	// WithField returns a clone of the context, but with added field with key
	// "key" and value "value".
	//
	// See also WithTag.
	WithField(key string, value interface{}) Context

	// WithFields returns a clone of the context, but with added fields with
	// key and values according to map "Fields".
	//
	// See also WithTag.
	WithFields(Fields) Context

	// Until works similar to Done(), but it is possible to specify specific
	// signal to wait for.
	//
	// If err is nil, then waits for any event.
	Until(err error) <-chan struct{}

	// StdCtxUntil is the same as Until, but returns a standard context
	// instead of a channel.
	StdCtxUntil(err error) context.Context

	// IsSignaledWith returns true if the context received a cancel
	// or a notification signal equals to any of passed ones.
	//
	// If errs is empty, then returns true if the context received any
	// cancel or notification signal.
	IsSignaledWith(errs ...error) bool

	// Notifications returns all the received notifications (including events
	// received by parents).
	//
	// This is a read-only value, do not modify it.
	Notifications() []error

	// Recover is use instead of standard "recover()" to also log the panic.
	Recover() interface{}

	// private:

	resetEventHandler()
	addEventHandler() *eventHandler
	addValue(key, value interface{})
	cloneWithStdContext(context.Context) Context
}

// TimeSpan is the object represents the time span to be reported by a Tracer.
type TimeSpan interface {
	// Finish sets the end time of the span to time.Now() and sends
	// the time span to the log of the Tracer.
	Finish() time.Duration
}

// Tracer is a handler responsible to track time spans.
//
// Is supposed to be used this way:
//
//     defer ctx.Tracer().StartSpan("some label here").Finish()
type Tracer interface {
	// StartSpan creates a time span to be reported (if Finish will be called)
	// which starts counting time since the moment StartSpan was called.
	StartSpan(label string) TimeSpan

	// WithField returns a Tracer with an added field to be reported with the time span (when Finish will be called).
	WithField(key string, value interface{}) Tracer

	// WithField returns a Tracer with added fields to be reported with the time span (when Finish will be called).
	WithFields(Fields) Tracer
}

// Metrics is a handler of metrics (like Prometheus, ODS)
type Metrics = metrics.Metrics

var _ context.Context = &ctxValue{}

type ctxValue struct {
	mutationSyncer  sync.Once
	traceIDValue    TraceID
	loggerInstance  *Logger
	metricsInstance *Metrics
	tracerInstance  *Tracer
	pendingFields   fields.PendingFields
	pendingTags     fields.PendingFields

	valuesHandler
	*eventHandler

	// parent is used only to warn the GC to do not run finalizers for eventHandlers-s
	// of parents.
	parent *ctxValue
}

var (
	background = NewContext(context.Background(), "", nil, nil, nil, nil, nil)
)

// Background is analog of standard context.Context which returns just a simple dummy context which does nothing.
func Background() Context {
	return background
}

var (
	hostname string
	curUser  *user.User
)

func init() {
	hostname, _ = os.Hostname()
	curUser, _ = user.Current()
}

// Extend converts a standard context to an extended one
func Extend(parent context.Context) Context {
	return NewContext(parent, "", nil, nil, nil, nil, nil)
}

// NewContext is a customizable constructor of a context.
//
// It is not intended to be called by an user not familiar with this package,
// there are special helpers for that, see for example bundles.NewContextWithLogrus.
func NewContext(
	stdCtx context.Context,
	traceID TraceID,
	loggerInstance Logger,
	metrics Metrics,
	tracer Tracer,
	tags Fields,
	fields Fields,
) Context {
	if traceID == "" {
		traceID = NewTraceID()
	}
	if loggerInstance == nil {
		loggerInstance = logger.Dummy()
	}

	ctx := &ctxValue{
		traceIDValue:    traceID,
		loggerInstance:  &loggerInstance,
		metricsInstance: &metrics,
		tracerInstance:  &tracer,
	}

	if stdCtx != nil && stdCtx != context.Background() {
		ctx = ctx.cloneWithStdContext(stdCtx).(*ctxValue)
	}

	if tags == nil {
		tags = Fields{}
		if buildinfo.BuildMode != "" {
			tags["buildMode"] = buildinfo.BuildMode
		}
		if buildinfo.BuildDate != "" {
			tags["buildDate"] = buildinfo.BuildDate
		}
		if buildinfo.Revision != "" {
			tags["revision"] = buildinfo.Revision
		}
		if DefaultLogHostname && hostname != "" {
			tags["hostname"] = hostname
		}
		if DefaultLogUsername && curUser != nil {
			tags["username"] = curUser.Name
		}
	}
	if len(tags) > 0 {
		ctx.pendingTags.AddMultiple(tags)
	}

	if fields == nil {
		fields = Fields{}
	}
	if DefaultLogTraceID {
		fields["traceID"] = traceID
	}
	ctx.pendingFields.AddMultiple(fields)

	return ctx
}

type CancelFunc = context.CancelFunc

func (ctx *ctxValue) clone() *ctxValue {
	return &ctxValue{
		traceIDValue:    ctx.traceIDValue,
		loggerInstance:  ctx.loggerInstance,
		metricsInstance: ctx.metricsInstance,
		tracerInstance:  ctx.tracerInstance,
		pendingFields:   ctx.pendingFields.Clone(),
		pendingTags:     ctx.pendingTags.Clone(),
		valuesHandler:   ctx.valuesHandler,
		eventHandler:    ctx.eventHandler,
		parent:          ctx,
	}
}

// Clone returns a derivative context in a new scope. Modifications of
// this context will not affect the original one.
func (ctx *ctxValue) Clone() Context {
	return ctx.clone()
}

// TraceID returns the tracing ID of the context. The tracing ID is
// the passing-through ID which is used to identify a flow across multiple
// services.
func (ctx *ctxValue) TraceID() TraceID {
	return ctx.traceIDValue
}

// WithTraceID returns a derivative context with passed tracing ID.
func (ctx *ctxValue) WithTraceID(traceID TraceID) Context {
	ctxClone := ctx.clone()
	ctxClone.traceIDValue = traceID
	return ctxClone.WithTag("traceID", traceID)
}

func (ctx *ctxValue) considerPendingTags() {
	if ctx.pendingTags.Slice == nil {
		return
	}

	pendingTags := ctx.pendingTags.Compile()
	ctx.pendingTags.Slice = nil
	ctx.pendingTags.IsReadOnly = false

	if loggerInstance := ctx.loadLoggerInstance(); loggerInstance != nil {
		ctx.storeLoggerInstance(loggerInstance.WithFields(pendingTags))
	}
	if tracerInstance := ctx.loadTracerInstance(); tracerInstance != nil {
		ctx.storeTracerInstance(tracerInstance.WithFields(pendingTags))
	}
	if metricsInstance := ctx.loadMetricsInstance(); metricsInstance != nil {
		ctx.storeMetricsInstance(metricsInstance.WithTags(pendingTags))
	}
}

func (ctx *ctxValue) considerPendingFields() {
	if ctx.pendingFields.Slice == nil {
		return
	}

	pendingFields := ctx.pendingFields.Compile()
	ctx.pendingFields.Slice = nil
	ctx.pendingFields.IsReadOnly = false

	if loggerInstance := ctx.loadLoggerInstance(); loggerInstance != nil {
		ctx.storeLoggerInstance(loggerInstance.WithFields(pendingFields))
	}
	if tracerInstance := ctx.loadTracerInstance(); tracerInstance != nil {
		ctx.storeTracerInstance(tracerInstance.WithFields(pendingFields))
	}
}

func (ctx *ctxValue) considerPending() {
	ctx.considerPendingTags()
	ctx.considerPendingFields()
}

// Logger returns a Logger.
func (ctx *ctxValue) Logger() Logger {
	loggerInstance := ctx.loadLoggerInstance()
	if loggerInstance == nil {
		return nil
	}
	ctx.mutationSyncer.Do(func() {
		ctx.considerPending()
		loggerInstance = ctx.loadLoggerInstance()
	})
	return loggerInstance
}

// WithLogger returns a derivative context with the Logger replaced with
// the passed one.
func (ctx *ctxValue) WithLogger(logger Logger) Context {
	ctxClone := ctx.clone()
	ctxClone.loggerInstance = &logger
	return ctxClone
}

// Metrics returns a Metrics handler.
func (ctx *ctxValue) Metrics() Metrics {
	metricsInstance := ctx.loadMetricsInstance()
	if metricsInstance == nil {
		return nil
	}
	ctx.mutationSyncer.Do(func() {
		ctx.considerPending()
		metricsInstance = ctx.loadMetricsInstance()
	})
	return metricsInstance
}

// WithMetrics returns a derivative context with the Metrics handler replaced with
// the passed one.
func (ctx *ctxValue) WithMetrics(metrics Metrics) Context {
	ctxClone := ctx.clone()
	ctxClone.metricsInstance = &metrics
	return ctxClone
}

// Tracer returns a Tracer handler.
func (ctx *ctxValue) Tracer() Tracer {
	tracerInstance := ctx.loadTracerInstance()
	if tracerInstance == nil {
		return dummyTracerInstance
	}
	ctx.mutationSyncer.Do(func() {
		ctx.considerPending()
		tracerInstance = ctx.loadTracerInstance()
	})
	return tracerInstance
}

// WithTracer returns a derivative context with the Tracer handler replaced with
// the passed one.
func (ctx *ctxValue) WithTracer(tracer Tracer) Context {
	ctxClone := ctx.clone()
	ctxClone.tracerInstance = &tracer
	return ctxClone
}

// WithTracer returns a derivative context with an added tag.
func (ctx *ctxValue) WithTag(key string, value interface{}) Context {
	ctxClone := ctx.clone()
	ctxClone.pendingTags.AddOne(key, value)
	return ctxClone
}

// WithTracer returns a derivative context with added tags.
func (ctx *ctxValue) WithTags(fields Fields) Context {
	ctxClone := ctx.clone()
	ctxClone.pendingTags.AddMultiple(fields)
	return ctxClone
}

// WithTracer returns a derivative context with an added field.
func (ctx *ctxValue) WithField(key string, value interface{}) Context {
	ctxClone := ctx.clone()
	ctxClone.pendingFields.AddOne(key, value)
	return ctxClone
}

// WithTracer returns a derivative context with added fields.
func (ctx *ctxValue) WithFields(fields Fields) Context {
	ctxClone := ctx.clone()
	ctxClone.pendingFields.AddMultiple(fields)
	return ctxClone
}

// Recover if supposed to be used in defer-s to log and stop panics.
func (ctx *ctxValue) Recover() interface{} {
	// TODO: this is a very naive implementation and there's no way to
	//       inject another handler, yet.
	//       It makes sense to design and introduce a Sentry-like handling
	//       of panics.
	r := recover()
	if r != nil {
		b := make([]byte, 65536)
		n := runtime.Stack(b, false)
		b = b[:n]
		ctx.Errorf("received panic: %v\n%s\n", r, b)
	}
	return r
}

func (ctx *ctxValue) Value(key interface{}) interface{} {
	if ctx.valuesHandler == nil {
		return nil
	}
	return ctx.valuesHandler.Value(key)
}

// LoggerFrom returns a logger from a context.Context if can find any. And
// returns a dummy logger (which does nothing) if wasn't able to find any.
func LoggerFrom(stdCtx context.Context) Logger {
	if stdCtx == nil {
		return logger.Dummy()
	}

	ctx, ok := stdCtx.(Context)
	if !ok {
		return logger.Dummy()
	}

	return ctx.Logger()
}

// StdCtxWaitFor is the same as Until, but returns a standard context
// instead of a channel.
func (ctx *ctxValue) StdCtxUntil(err error) context.Context {
	child := ctx.clone()
	if child.eventHandler == nil {
		return child
	}

	child.eventHandler = nil
	h := child.addEventHandler()

	garbageCollected := make(chan struct{})
	runtime.SetFinalizer(&child.cancelSignal, func(*ctxValue) {
		close(garbageCollected)
	})

	go func() {
		select {
		case <-garbageCollected:
			return
		case <-ctx.Until(err):
			h.cancel(ErrCanceled)
		}
	}()

	return child
}
