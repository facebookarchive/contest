// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetmanager

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/event/testevent"
)

// EventEmitter is just a wrapper around testevent.Emitter to make sure
// if emitted event name is correct.
type EventEmitter struct {
	TestEventEmitter testevent.Emitter
}

// Emit implements testevent.Emitter.
func (emitter EventEmitter) Emit(event testevent.Data) error {
	if event.EventName != EventName {
		return fmt.Errorf("invalid event name: '%s' != '%s'", event.EventName, EventName)
	}

	return emitter.TestEventEmitter.Emit(event)
}
