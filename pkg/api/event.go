// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package api

import (
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

// EventType identifies an API event type.
type EventType uint16

// EventRequestor identifies who is sending a request. This is set on the client
// side, but can be validated and overridden in the listener if necessary.
// This is *not* authentication, it's just the client declaring who they are, and
// obviously clients can change this field to whatever they want.
type EventRequestor string

func (e EventType) String() string {
	if name, ok := eventTypeNames[e]; ok {
		return name
	}
	return "unknown_event"
}

var eventTypeNames = map[EventType]string{
	EventTypeStart:  "event_type_start",
	EventTypeStatus: "event_type_status",
	EventTypeStop:   "event_type_stop",
	EventTypeRetry:  "event_type_retry",
	EventTypeError:  "event_type_error",
}

// list of existing API event types.
const (
	EventTypeStart EventType = iota
	EventTypeStatus
	EventTypeStop
	EventTypeRetry
	EventTypeError
)

// Event represents an event that the API can generate. This is used by the API
// listener to enable event handling.
type Event struct {
	Type EventType
	Err  error
	Msg  EventMsg
	// RespCh is a channel where the JobManager can send the responses back to
	// what generated the event. E.g. if a job status is requested, the answer
	// goes back to the caller in an EventResponse via this channel.
	RespCh chan *EventResponse
}

// EventMsg defines various event messages for different event types.
// Error events have no associated message, the error information is set in the
// Err attribute.
type EventMsg interface {
	Requestor() EventRequestor
}

// EventStartMsg contains the arguments for an event of type Start.
type EventStartMsg struct {
	requestor     EventRequestor
	TestID        string
	NumRuns       uint32
	JobDescriptor string
}

// Requestor returns the requestor of the API call as reported by the client.
func (e EventStartMsg) Requestor() EventRequestor { return e.requestor }

// EventStatusMsg contains the arguments for an event of type Status.
type EventStatusMsg struct {
	requestor EventRequestor
	JobID     types.JobID
}

// Requestor returns the requestor of the API call as reported by the client.
func (e EventStatusMsg) Requestor() EventRequestor { return e.requestor }

// EventStopMsg contains the arguments for an event of type Stop.
type EventStopMsg struct {
	requestor EventRequestor
	JobID     types.JobID
}

// Requestor returns the requestor of the API call as reported by the client.
func (e EventStopMsg) Requestor() EventRequestor { return e.requestor }

// EventRetryMsg contains the arguments for an event of type Retry.
type EventRetryMsg struct {
	requestor EventRequestor
	JobID     types.JobID
}

// Requestor returns the requestor of the API call as reported by the client.
func (e EventRetryMsg) Requestor() EventRequestor { return e.requestor }

// EventResponse is a response to an EventMsg.
type EventResponse struct {
	Requestor EventRequestor
	JobID     types.JobID
	Err       error
	Status    *job.Status
}
