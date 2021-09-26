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

type Severity string

const (
	SeverityInfo    = Severity("INFO")
	SeverityDebug   = Severity("DEBUG")
	SeverityWarning = Severity("WARNING")
	SeverityError   = Severity("ERROR")
	SeverityFatal   = Severity("FATAL")
)

type Status string

const (
	StatusUnkown   = Status("UNKNOWN")
	StatusComplete = Status("COMPLETE")
	StatusError    = Status("ERROR")
	StatusSkipped  = Status("SKIPPED")
)

type Result string

const (
	ResultPass = Result("PASS")
	ResultFail = Result("FAIL")
	ResultNA   = Result("NOT_APPLICABLE")
)

// TODO: these should just be temporary until the go:generate tool
// TODO: this should also mean refactoring all the parser code
type Log struct {
	Severity Severity `json:"severity,omitempty"`
	Text     string   `json:"text,omitempty"`
}

type RunStart struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type RunEnd struct {
	Name   string `json:"name,omitempty"`
	Status Status `json:"status,omitempty"`
	Result Result `json:"result,omitempty"`
}

type RunArtifact struct {
	RunStart *RunStart `json:"testRunStart,omitempty"`
	RunEnd   *RunEnd   `json:"testRunEnd,omitempty"`
	Log      *Log      `json:"log,omitempty"`
}

type StepStart struct {
	Name string `json:"name,omitempty"`
}

type StepEnd struct {
	Name   string `json:"name,omitempty"`
	Status Status `json:"status,omitempty"`
}

type StepArtifact struct {
	StepId string `json:"testStepId,omitempty"`

	StepStart *StepStart `json:"testStepStart,omitempty"`
	StepEnd   *StepEnd   `json:"testStepEnd,omitempty"`
	Log       *Log       `json:"log,omitempty"`
}

type OCPRoot struct {
	SequenceNumber int           `json:"sequenceNumber"`
	Timestamp      string        `json:"timestamp"`
	RunArtifact    *RunArtifact  `json:"testRunArtifact,omitempty"`
	StepArtifact   *StepArtifact `json:"testStepArtifact,omitempty"`
}

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

// TODO: check if there can be multiple runs in the same output
type OCPState struct {
	RunEnd *RunEnd
}

func (s OCPState) Error() error {
	if s.RunEnd == nil {
		return fmt.Errorf("did not see a complete run")
	}

	if s.RunEnd.Result != ResultPass {
		// TODO: add a concat log of errors?
		return fmt.Errorf("test failed")
	}

	return nil
}

type OCPEventParser struct {
	target *target.Target
	tee    testevent.Emitter
	state  OCPState
}

func NewOCPEventParser(target *target.Target, ev testevent.Emitter) *OCPEventParser {
	return &OCPEventParser{
		target: target,
		tee:    ev,
		state:  OCPState{},
	}
}

func (ep *OCPEventParser) emit(ctx xcontext.Context, name event.Name, data []byte) error {
	json := json.RawMessage(data)
	event := testevent.Data{
		EventName: name,
		Target:    ep.target,
		Payload:   &json,
	}
	return ep.tee.Emit(ctx, event)
}

func (ep *OCPEventParser) parseRun(ctx xcontext.Context, node *RunArtifact, root *OCPRoot) error {
	if node.RunStart != nil {
		type startdata struct {
			SequenceNumber int    `json:"sequenceNumber"`
			Timestamp      string `json:"timestamp"`
			Name           string `json:"name,omitempty"`
			Version        string `json:"version,omitempty"`
		}

		data, err := json.Marshal(&startdata{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			Name:           node.RunStart.Name,
			Version:        node.RunStart.Version,
		})
		if err != nil {
			return err
		}

		return ep.emit(ctx, TestStartEvent, data)
	}

	if node.RunEnd != nil {
		if node.RunEnd.Status == StatusComplete {
			ep.state.RunEnd = node.RunEnd
		}

		type enddata struct {
			SequenceNumber int    `json:"sequenceNumber"`
			Timestamp      string `json:"timestamp"`
			Name           string `json:"name,omitempty"`
			Status         string `json:"status,omitempty"`
			Result         string `json:"result,omitempty"`
		}

		data, err := json.Marshal(&enddata{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			Name:           node.RunEnd.Name,
			Status:         string(node.RunEnd.Status),
			Result:         string(node.RunEnd.Result),
		})
		if err != nil {
			return err
		}

		return ep.emit(ctx, TestEndEvent, data)
	}

	if node.Log != nil {
		type logdata struct {
			SequenceNumber int    `json:"sequenceNumber"`
			Timestamp      string `json:"timestamp"`
			Severity       string `json:"severity,omitempty"`
			Message        string `json:"text,omitempty"`
		}

		data, err := json.Marshal(&logdata{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			Severity:       string(node.Log.Severity),
			Message:        node.Log.Text,
		})
		if err != nil {
			return err
		}

		return ep.emit(ctx, TestLogEvent, data)
	}

	return nil
}

func (ep *OCPEventParser) parseStep(ctx xcontext.Context, node *StepArtifact, root *OCPRoot) error {
	if node.StepStart != nil {
		type startdata struct {
			SequenceNumber int    `json:"sequenceNumber"`
			Timestamp      string `json:"timestamp"`
			StepId         string `json:"stepId"`
			Name           string `json:"name"`
		}

		data, err := json.Marshal(&startdata{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			StepId:         node.StepId,
			Name:           node.StepStart.Name,
		})
		if err != nil {
			return err
		}

		return ep.emit(ctx, StepStartEvent, data)
	}

	if node.StepEnd != nil {
		type enddata struct {
			SequenceNumber int    `json:"sequenceNumber"`
			Timestamp      string `json:"timestamp"`
			StepId         string `json:"stepId"`
			Name           string `json:"name"`
			Status         string `json:"status"`
		}

		data, err := json.Marshal(&enddata{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			StepId:         node.StepId,
			Name:           node.StepEnd.Name,
			Status:         string(node.StepEnd.Status),
		})
		if err != nil {
			return err
		}

		return ep.emit(ctx, StepEndEvent, data)
	}

	if node.Log != nil {
		type logdata struct {
			SequenceNumber int    `json:"sequenceNumber"`
			Timestamp      string `json:"timestamp"`
			StepId         string `json:"stepId"`
			Severity       string `json:"severity,omitempty"`
			Message        string `json:"text,omitempty"`
		}

		data, err := json.Marshal(&logdata{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			StepId:         node.StepId,
			Severity:       string(node.Log.Severity),
			Message:        node.Log.Text,
		})
		if err != nil {
			return err
		}

		return ep.emit(ctx, StepLogEvent, data)
	}

	return nil
}

func (ep *OCPEventParser) Parse(ctx xcontext.Context, root *OCPRoot) error {
	if root.RunArtifact != nil {
		return ep.parseRun(ctx, root.RunArtifact, root)
	}

	if root.StepArtifact != nil {
		return ep.parseStep(ctx, root.StepArtifact, root)
	}

	return nil
}

func (ep *OCPEventParser) Error() error {
	return ep.state.Error()
}
