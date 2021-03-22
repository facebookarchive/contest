// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"context"
	"errors"
)

// Canceled is returned by Context.Err when the context was canceled
var Canceled = context.Canceled

// DeadlineExceeded is returned by Context.Err when the context reached
// the deadline.
var DeadlineExceeded = context.DeadlineExceeded

// Paused is returned by Context.Err when the context was paused
var Paused = errors.New("job is paused")
