// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logadapter

// ErrScubaQueueIsFull is returned when a message wasn't able to be sent
// to scuba because the queue (of messages for scuba) is already full.
type ErrScubaQueueIsFull struct{}

// Error implements error.
func (err ErrScubaQueueIsFull) Error() string {
	return "unable to send to scuba: the queue is full"
}
