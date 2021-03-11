// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package internal of logger unifies different types of loggers into
// interfaces Logger. For example it allows to upgrade simple fmt.Printf
// to be a fully functional Logger. Therefore multiple wrappers are implemented
// here to provide different functions which could be missing in some loggers.
package internal

// WrapMinimalLogger wraps a MinimalLogger to provide a Logger.
func WrapMinimalLogger(logger MinimalLogger) Logger {
	return ExtendWrapper{
		MinimalLogger: logger,
		Prefix:        "",
		CurLevel:      LevelWarning,
	}
}
