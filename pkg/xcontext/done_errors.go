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
