// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLogLevel(t *testing.T) {
	tryParseLevel := func(expectedLevel Level, input string) {
		level, err := ParseLogLevel(input)
		if assert.NoError(t, err, input) {
			assert.Equal(t, expectedLevel, level, input)
		}

		reparsedLevel, err := ParseLogLevel(level.String())
		if assert.NoError(t, err, input) {
			assert.Equal(t, level, reparsedLevel, input)
		}
	}
	tryParseLevel(LevelDebug, "debug")
	tryParseLevel(LevelDebug, "dEbug")
	tryParseLevel(LevelInfo, "info")
	tryParseLevel(LevelWarning, "warn")
	tryParseLevel(LevelError, "error")
	tryParseLevel(LevelPanic, "panic")
	tryParseLevel(LevelFatal, "fatal")
	level, err := ParseLogLevel("unknown")
	assert.Error(t, err)
	assert.Equal(t, LevelUndefined, level)
}
