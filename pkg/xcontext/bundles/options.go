// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package bundles

import (
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Option is a modifier of what context to construct.
type Option interface {
	apply(cfg *Config)
}

// OptionLoggerReportCaller defines if it is required to determine
// source-code file name and line and report it to logs.
// It negatively affects performance of logging.
type OptionLoggerReportCaller bool

func (opt OptionLoggerReportCaller) apply(cfg *Config) {
	cfg.LoggerReportCaller = bool(opt)
}

// OptionTracerReportCaller the same as OptionLoggerReportCaller, but
// for a tracer.
type OptionTracerReportCaller bool

func (opt OptionTracerReportCaller) apply(cfg *Config) {
	cfg.TracerReportCaller = bool(opt)
}

// LogFormat is a selector of an output format.
type LogFormat int

const (
	// LogFormatPlainText means to write logs as a plain text.
	LogFormatPlainText = LogFormat(iota)

	// LogFormatJSON means to write logs as JSON objects.
	LogFormatJSON
)

// OptionLogFormat defines the format for logs.
type OptionLogFormat LogFormat

func (opt OptionLogFormat) apply(cfg *Config) {
	cfg.Format = LogFormat(opt)
}

// OptionTracer defines if a tracer should be added to the context.
type OptionTracer struct {
	xcontext.Tracer
}

func (opt OptionTracer) apply(cfg *Config) {
	cfg.Tracer = opt.Tracer
}

// OptionTimestampFormat defines the format of timestamps while logging.
type OptionTimestampFormat string

func (opt OptionTimestampFormat) apply(cfg *Config) {
	cfg.TimestampFormat = string(opt)
}

// Config is a configuration state resulted from Option-s.
type Config struct {
	LoggerReportCaller bool
	TracerReportCaller bool
	TimestampFormat    string
	VerboseCaller      bool
	Tracer             xcontext.Tracer
	Format             LogFormat
}

// GetConfig processes passed Option-s and returns the resulting state as Config.
func GetConfig(opts ...Option) Config {
	cfg := Config{
		LoggerReportCaller: true,
		TracerReportCaller: true,
	}
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	return cfg
}
