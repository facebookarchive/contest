// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package exec

import (
	"fmt"

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
	ev     testevent.Emitter
	state  OCPState
}

func NewOCPEventParser(target *target.Target, ev testevent.Emitter) *OCPEventParser {
	return &OCPEventParser{
		target: target,
		ev:     ev,
		state:  OCPState{},
	}
}

func (p *OCPEventParser) parseRun(ctx xcontext.Context, node *RunArtifact, root *OCPRoot) error {
	if node.RunStart != nil {
		payload := testStartEventPayload{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			Name:           node.RunStart.Name,
			Version:        node.RunStart.Version,
		}
		return emitEvent(ctx, TestStartEvent, payload, p.target, p.ev)
	}

	if node.RunEnd != nil {
		if node.RunEnd.Status == StatusComplete {
			p.state.RunEnd = node.RunEnd
		}

		payload := testEndEventPayload{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			Name:           node.RunEnd.Name,
			Status:         string(node.RunEnd.Status),
			Result:         string(node.RunEnd.Result),
		}
		return emitEvent(ctx, TestEndEvent, payload, p.target, p.ev)
	}

	if node.Log != nil {
		payload := testLogEventPayload{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			Severity:       string(node.Log.Severity),
			Message:        node.Log.Text,
		}
		return emitEvent(ctx, TestLogEvent, payload, p.target, p.ev)
	}

	return nil
}

func (p *OCPEventParser) parseStep(ctx xcontext.Context, node *StepArtifact, root *OCPRoot) error {
	if node.StepStart != nil {
		payload := stepStartEventPayload{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			StepId:         node.StepId,
			Name:           node.StepStart.Name,
		}
		return emitEvent(ctx, StepStartEvent, payload, p.target, p.ev)
	}

	if node.StepEnd != nil {
		payload := stepEndEventPayload{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			StepId:         node.StepId,
			Name:           node.StepEnd.Name,
			Status:         string(node.StepEnd.Status),
		}
		return emitEvent(ctx, StepEndEvent, payload, p.target, p.ev)
	}

	if node.Log != nil {
		payload := stepLogEventPayload{
			SequenceNumber: root.SequenceNumber,
			Timestamp:      root.Timestamp,
			StepId:         node.StepId,
			Severity:       string(node.Log.Severity),
			Message:        node.Log.Text,
		}
		return emitEvent(ctx, StepLogEvent, payload, p.target, p.ev)
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
