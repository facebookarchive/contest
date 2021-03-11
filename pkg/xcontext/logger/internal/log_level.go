// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package internal of logger unifies different types of loggers into
// interfaces Logger. For example it allows to upgrade simple fmt.Printf
// to be a fully functional Logger. Therefore multiple wrappers are implemented
// here to provide different functions which could be missing in some loggers.
package internal

import (
	"fmt"
	"strings"
)

// Level is used to define severity of messages to be reported.
type Level int

const (
	// LevelUndefined is the erroneous value of log-level which corresponds
	// to zero-value.
	LevelUndefined = Level(iota)

	// LevelFatal will report about Fatalf-s only.
	LevelFatal

	// LevelPanic will report about Panicf-s and Fatalf-s only.
	LevelPanic

	// LevelError will report about Errorf-s, Panicf-s, ...
	LevelError

	// LevelWarning will report about Warningf-s, Errorf-s, ...
	LevelWarning

	// LevelInfo will report about Infof-s, Warningf-s, ...
	LevelInfo

	// LevelDebug will report about Debugf-s, Infof-s, ...
	LevelDebug
)

// String just implements fmt.Stringer, flag.Value and pflag.Value.
func (logLevel Level) String() string {
	switch logLevel {
	case LevelUndefined:
		return "undefined"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarning:
		return "warning"
	case LevelError:
		return "error"
	case LevelPanic:
		return "panic"
	case LevelFatal:
		return "fatal"
	}
	return "unknown"
}

// Set updates the logging level values based on the passed string value.
// This method just implements flag.Value and pflag.Value.
func (logLevel *Level) Set(value string) error {
	newLogLevel, err := ParseLogLevel(value)
	if err != nil {
		return err
	}
	*logLevel = newLogLevel
	return nil
}

// Type just implements pflag.Value.
func (logLevel *Level) Type() string {
	return "Level"
}

// ParseLogLevel parses incoming string into a Level and returns
// LevelUndefined with an error if an unknown logging level was passed.
func ParseLogLevel(in string) (Level, error) {
	switch strings.ToLower(in) {
	case "d", "debug":
		return LevelDebug, nil
	case "i", "info":
		return LevelInfo, nil
	case "w", "warn", "warning":
		return LevelWarning, nil
	case "e", "err", "error":
		return LevelError, nil
	case "p", "panic":
		return LevelPanic, nil
	case "f", "fatal":
		return LevelFatal, nil
	}
	var allowedValues []string
	for logLevel := LevelFatal; logLevel <= LevelDebug; logLevel++ {
		allowedValues = append(allowedValues, logLevel.String())
	}
	return LevelUndefined, fmt.Errorf("unknown logging level '%s', known values are: %s",
		in, strings.Join(allowedValues, ", "))
}
