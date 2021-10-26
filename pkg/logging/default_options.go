// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

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
