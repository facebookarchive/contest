// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package api

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/facebookincubator/contest/pkg/types"
)

// CurrentAPIVersion is the current version of the API that the clients must be
// able to speak in order to communicate with the server. Versioning starts at
// 1, while 0 is to be considered an error indicator.
const CurrentAPIVersion uint32 = 5

// DefaultEventTimeout is the default time to wait for sending or receiving an
// event on the events channel.
var DefaultEventTimeout = 3 * time.Second

// The API structure implements the communication between clients and the
// JobManager. It enables several operations like starting, stopping,
// retrying a job, and getting a job status.
type API struct {
	// The events channel is used to route API events between clients and the
	// JobManager. It is not necessary to close it explicitly as it will be
	// garbage-collected when the API structure in the client goes out of scope.
	Events chan *Event
	// serverIDFunc is used by ServerID() to return a custom server ID in API
	// responses.
	serverIDFunc func() string
}

// New returns an initialized instance of an API struct with a default server ID
// generation function.
func New() *API {
	return NewWithServerIDFunc(nil)
}

// NewWithServerIDFunc is like New, but it lets the user specify a function to
// generate the server ID.
func NewWithServerIDFunc(serverIDFunc func() string) *API {
	return &API{
		Events:       make(chan *Event),
		serverIDFunc: serverIDFunc,
	}
}

// ServerID returns the Server ID to be used in responses. A custom server ID
// generation function can be passed to New().
func (a API) ServerID() string {
	if a.serverIDFunc != nil {
		return a.serverIDFunc()
	}
	hn, err := os.Hostname()
	if err != nil {
		return "<unknown>"
	}
	return hn
}

// newResponse returns a new Response object with type and server ID set. The
// Data field has to be set by the user.
func (a API) newResponse(rtype ResponseType) Response {
	return Response{
		Type:     rtype,
		ServerID: a.ServerID(),
	}
}

// Version returns the version of the API. It's the client's responsibility
// to check whether it can talk the right API. If the client speaks an
// incompatible version of the API that the server doesn't understand, it's
// the server's responsibility to return an error upon API calls.
func (a API) Version() Response {
	// NOTE: backward-compatibility should be handled by a proxy endpoint that
	// speaks the same API, and will detect the version and redirect to the
	// appropriate backend. This will simplify the way migrations are carried
	// over.
	resp := a.newResponse(ResponseTypeVersion)
	resp.Data = ResponseDataVersion{
		Version: CurrentAPIVersion,
	}
	return resp
}

// SendEvent sends an Event object on the event channel, without waiting for a
// reply. If the send doesn't complete within the timeout, an error is returned.
func (a *API) SendEvent(ev *Event, timeout *time.Duration) error {
	if ev.Msg.Requestor() == "" {
		return errors.New("requestor cannot be empty")
	}
	to := DefaultEventTimeout
	if timeout != nil {
		to = *timeout
	}
	select {
	case a.Events <- ev:
		return nil
	case <-time.After(to):
		return fmt.Errorf("sending event timed out after %v", timeout)
	}
}

// SendReceiveEvent sends an Event object on the event channel, and waits for a reply
// from the consumer. The timeout is used once for the send, and once for the
// receive, it's not a cumulative timeout.
func (a *API) SendReceiveEvent(ev *Event, timeout *time.Duration) (*EventResponse, error) {
	to := DefaultEventTimeout
	if timeout != nil {
		to = *timeout
	}
	// send
	if err := a.SendEvent(ev, &to); err != nil {
		return nil, err
	}
	// receive
	var resp *EventResponse
	select {
	case resp = <-ev.RespCh:
		return resp, nil
	case <-time.After(to):
		return nil, fmt.Errorf("time out waiting for response after %v", timeout)
	}
}

// Start requests to create a new test job, as described by the job descriptor.
// A job descriptor may contain multiple tests, which will be run sequentially,
// not in parallel. If you need parallelism, you need to submit multiple
// independent jobs. If you need coordination across jobs, you need to write
// your own synchronization plugins that use external means (e.g. the events
// API), but no inter-job synchronization is implemented in the framework
// itself. This is intentional, to avoid overcomplicating the orchestration
// for a few edge cases.
// Each job descriptor must be JSON-encoded, and will be deserialized in a
// `contest.JobDescriptor` object by the JobManager.
// This method must return a unique job ID, that can be used for various
// operations via the API, e.g. getting the job status or stopping it.
// This method should return an error if the job description is malformed or
// invalid, and if the API version is incompatible.
func (a *API) Start(requestor EventRequestor, jobDescriptor string) (Response, error) {
	resp := a.newResponse(ResponseTypeStart)
	ev := &Event{
		Type:     EventTypeStart,
		ServerID: resp.ServerID,
		Msg: EventStartMsg{
			requestor:     requestor,
			JobDescriptor: jobDescriptor,
		},
		RespCh: make(chan *EventResponse, 1),
	}
	respEv, err := a.SendReceiveEvent(ev, nil)
	if err != nil {
		return resp, err
	}
	resp.Data = ResponseDataStart{
		JobID: respEv.JobID,
	}
	resp.Err = respEv.Err
	return resp, nil
}

// Stop requests a job cancellation by the given job ID.
func (a *API) Stop(requestor EventRequestor, jobID types.JobID) (Response, error) {
	resp := a.newResponse(ResponseTypeStop)
	ev := &Event{
		Type:     EventTypeStop,
		ServerID: resp.ServerID,
		Msg: EventStopMsg{
			requestor: requestor,
			JobID:     jobID,
		},
		RespCh: make(chan *EventResponse, 1),
	}
	respEv, err := a.SendReceiveEvent(ev, nil)
	if err != nil {
		return resp, err
	}
	resp.Data = ResponseDataStop{}
	resp.Err = respEv.Err
	return resp, nil
}

// Status polls the status of a job by its ID, and returns a contest.Status
//object
func (a *API) Status(requestor EventRequestor, jobID types.JobID) (Response, error) {
	resp := a.newResponse(ResponseTypeStatus)
	ev := &Event{
		Type:     EventTypeStatus,
		ServerID: resp.ServerID,
		Msg: EventStatusMsg{
			requestor: requestor,
			JobID:     jobID,
		},
		RespCh: make(chan *EventResponse, 1),
	}
	respEv, err := a.SendReceiveEvent(ev, nil)
	if err != nil {
		return resp, err
	}
	resp.Data = ResponseDataStatus{
		Status: respEv.Status,
	}
	resp.Err = respEv.Err
	return resp, nil
}

// Retry will retry a job identified by its ID, using the same job
// description. If the job is still running, an error is returned.
func (a *API) Retry(requestor EventRequestor, jobID types.JobID) (Response, error) {
	resp := a.newResponse(ResponseTypeRetry)
	ev := &Event{
		Type:     EventTypeRetry,
		ServerID: resp.ServerID,
		Msg: EventRetryMsg{
			requestor: requestor,
			JobID:     jobID,
		},
		RespCh: make(chan *EventResponse, 1),
	}
	respEv, err := a.SendReceiveEvent(ev, nil)
	if err != nil {
		return resp, err
	}
	resp.Data = ResponseDataRetry{
		// this is the job ID of the job to retry, not the new job ID
		JobID: jobID,
		// TODO this should set the new Job ID
		// NewJobID: ...
	}
	resp.Err = respEv.Err
	return resp, nil
}
