// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package exec

import (
	"encoding/json"
	"fmt"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// events that we may emit during the plugin's lifecycle
const (
	TestStartEvent = event.Name("TestStart")
	TestEndEvent   = event.Name("TestEnd")
	TestLogEvent   = event.Name("TestLog")

	StepStartEvent = event.Name("StepStart")
	StepEndEvent   = event.Name("StepEnd")
	StepLogEvent   = event.Name("StepLog")
)

// Events defines the events that a TestStep is allow to emit. Emitting an event
// that is not registered here will cause the plugin to terminate with an error.
var Events = []event.Name{
	TestStartEvent, TestEndEvent,
	TestLogEvent,
	StepStartEvent, StepEndEvent,
	StepLogEvent,
}

type testStartEventPayload struct {
	SequenceNumber int    `json:"sequenceNumber"`
	Timestamp      string `json:"timestamp"`
	Name           string `json:"name,omitempty"`
	Version        string `json:"version,omitempty"`
}

type testEndEventPayload struct {
	SequenceNumber int    `json:"sequenceNumber"`
	Timestamp      string `json:"timestamp"`
	Name           string `json:"name,omitempty"`
	Status         string `json:"status,omitempty"`
	Result         string `json:"result,omitempty"`
}

type testLogEventPayload struct {
	SequenceNumber int    `json:"sequenceNumber"`
	Timestamp      string `json:"timestamp"`
	Severity       string `json:"severity,omitempty"`
	Message        string `json:"text,omitempty"`
}

type stepStartEventPayload struct {
	SequenceNumber int    `json:"sequenceNumber"`
	Timestamp      string `json:"timestamp"`
	StepId         string `json:"stepId"`
	Name           string `json:"name"`
}

type stepEndEventPayload struct {
	SequenceNumber int    `json:"sequenceNumber"`
	Timestamp      string `json:"timestamp"`
	StepId         string `json:"stepId"`
	Name           string `json:"name"`
	Status         string `json:"status"`
}

type stepLogEventPayload struct {
	SequenceNumber int    `json:"sequenceNumber"`
	Timestamp      string `json:"timestamp"`
	StepId         string `json:"stepId"`
	Severity       string `json:"severity,omitempty"`
	Message        string `json:"text,omitempty"`
}

func emitEvent(ctx xcontext.Context, name event.Name, payload interface{}, tgt *target.Target, ev testevent.Emitter) error {
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cannot marshal payload for event '%s': %w", name, err)
	}

	msg := json.RawMessage(payloadData)
	data := testevent.Data{
		EventName: name,
		Target:    tgt,
		Payload:   &msg,
	}
	if err := ev.Emit(ctx, data); err != nil {
		return fmt.Errorf("cannot emit event EventCmdStart: %w", err)
	}

	return nil
}
