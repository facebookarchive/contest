// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
)

// TestEventEmitter implements Emitter interface from the testevent package
type TestEventEmitter struct {
	header testevent.Header
}

// TestEventFetcher implements the Fetcher interface from the testevent package
type TestEventFetcher struct {
}

// TestEventEmitterFetcher implements Emitter and Fetcher interface of the testevent package
type TestEventEmitterFetcher struct {
	TestEventEmitter
	TestEventFetcher
}

// Emit emits an event using the selected storage layer
func (e TestEventEmitter) Emit(data testevent.Data) error {
	event := testevent.Event{Header: &e.header, Data: &data, EmitTime: time.Now()}
	if err := storage.StoreTestEvent(event); err != nil {
		return fmt.Errorf("could not persist event data %v: %v", data, err)
	}
	return nil
}

// Fetch retrieves events based on QueryFields that are used to build a Query object for TestEvents
func (ev TestEventFetcher) Fetch(fields []testevent.QueryField) ([]testevent.Event, error) {
	eventQuery := testevent.Query{}
	for _, field := range fields {
		field(&eventQuery)
	}
	return storage.GetTestEvent(&eventQuery)
}

// NewTestEventEmitter creates a new Emitter object associated with a Header
func NewTestEventEmitter(header testevent.Header) testevent.Emitter {
	return TestEventEmitter{header: header}
}

// NewTestEventFetcher creates a new Fetcher object associated with a Header
func NewTestEventFetcher() testevent.Fetcher {
	return TestEventFetcher{}
}

// NewTestEventEmitterFetcher creates a new EmitterFetcher object associated with a Header
func NewTestEventEmitterFetcher(header testevent.Header) testevent.EmitterFetcher {
	return TestEventEmitterFetcher{
		TestEventEmitter{header: header},
		TestEventFetcher{},
	}
}

// FrameworkEventEmitter implements Emitter interface from the frameworkevent package
type FrameworkEventEmitter struct {
}

// FrameworkEventsFetcher implements the Fetcher interface from the frameworkevent package
type FrameworkEventFetcher struct {
}

// FrameworkEventEmitterFetcher implements Emitter and Fetcher interface from the frameworkevent package
type FrameworkEventEmitterFetcher struct {
	FrameworkEventEmitter
	FrameworkEventFetcher
}

// Emit emits an event using the selected storage engine
func (ev FrameworkEventEmitter) Emit(event frameworkevent.Event) error {
	if err := storage.StoreFrameworkEvent(event); err != nil {
		return fmt.Errorf("could not persist event %v: %v", event, err)
	}
	return nil
}

// Fetch retrieves events based on QueryFields that are used to build a Query object for FrameworkEvents
func (ev FrameworkEventFetcher) Fetch(fields []frameworkevent.QueryField) ([]frameworkevent.Event, error) {
	eventQuery := frameworkevent.Query{}
	for _, field := range fields {
		field(&eventQuery)
	}
	return storage.GetFrameworkEvent(&eventQuery)
}

// NewFrameworkEventEmitter creates a new Emmitter object for framework events
func NewFrameworkEventEmitter() FrameworkEventEmitter {
	return FrameworkEventEmitter{}
}

// NewFrameworkEventFetcher creates a new Fetcher object for framework events
func NewFrameworkEventFetcher() FrameworkEventFetcher {
	return FrameworkEventFetcher{}
}

// NewFrameworkEventEmitterFetcher creates a new EmitterFetcher object for framework events
func NewFrameworkEventEmitterFetcher() FrameworkEventEmitterFetcher {
	return FrameworkEventEmitterFetcher{
		FrameworkEventEmitter{},
		FrameworkEventFetcher{},
	}
}
