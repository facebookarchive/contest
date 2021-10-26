// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package internal of logger unifies different types of loggers into
// interfaces Logger. For example it allows to upgrade simple fmt.Printf
// to be a fully functional Logger. Therefore multiple wrappers are implemented
// here to provide different functions which could be missing in some loggers.
package internal

var (
	_ MinimalLoggerCompact = DummyLogger{}
)

// DummyLogger is just a logger which does nothing.
// To do not duplicate anything we just implement MinimalLoggerCompact
// and the rest methods could be added using wrappers (see logger.ConvertLogger).
type DummyLogger struct{}

// Logf implements MinimalLoggerCompact.
func (DummyLogger) Logf(_ Level, _ string, _ ...interface{}) {}
