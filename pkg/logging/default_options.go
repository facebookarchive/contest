package logging

import (
	"github.com/facebookincubator/contest/pkg/xcontext/bundles"
)

// DefaultOptions is a set options recommended to use by default.
func DefaultOptions() []bundles.Option {
	return []bundles.Option{
		bundles.OptionLogFormat(bundles.LogFormatPlainTextCompact),
		bundles.OptionTimestampFormat("2006-01-02T15:04:05.000Z07:00"),
	}
}
