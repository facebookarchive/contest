// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"context"
	"errors"
)

// ErrCanceled is returned by Context.Err when the context was canceled
var ErrCanceled = context.Canceled

// ErrDeadlineExceeded is returned by Context.Err when the context reached
// the deadline.
var ErrDeadlineExceeded = context.DeadlineExceeded

// ErrPaused is returned by Context.Err when the context was paused
var ErrPaused = errors.New("job is paused")
