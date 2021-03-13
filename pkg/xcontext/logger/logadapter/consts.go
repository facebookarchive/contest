// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logadapter

const (
	// ScubaQueueLength defines how many entries we can preserve in the queue
	// of messages to be sent to Scuba. If the queue is being overflowed, then
	// new messages are just being skipped (to do not block the application
	// due to logging).
	ScubaQueueLength = 1000
)
