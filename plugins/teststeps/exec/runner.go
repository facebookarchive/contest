// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package exec

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps/exec/transport"
)

type outcome error

type TargetRunner struct {
	ts *TestStep
	ev testevent.Emitter
}

func NewTargetRunner(ts *TestStep, ev testevent.Emitter) *TargetRunner {
	return &TargetRunner{
		ts: ts,
		ev: ev,
	}
}

func (r *TargetRunner) runWithOCP(
	ctx xcontext.Context, target *target.Target,
	transport transport.Transport, params stepParams,
) (outcome, error) {
	proc, err := transport.NewProcess(ctx, params.Bin.Path, params.Bin.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to create proc: %w", err)
	}

	stdout, err := proc.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to pipe stout: %w", err)
	}

	if err := proc.Start(ctx); err != nil {
		return nil, err
	}

	p := NewOCPEventParser(target, r.ev)
	dec := json.NewDecoder(stdout)
	for dec.More() {
		var root *OCPRoot
		if err := dec.Decode(&root); err != nil {
			ctx.Warnf("failed to decode ocp root: %w", err)
		}

		if err := p.Parse(ctx, root); err != nil {
			ctx.Warnf("failed to parse ocp root: %w", err)
		}
	}

	if err := proc.Wait(ctx); err != nil {
		return nil, fmt.Errorf("failed to wait on transport: %w", err)
	}

	return p.Error(), nil
}

func (r *TargetRunner) runAny(
	ctx xcontext.Context, target *target.Target,
	transport transport.Transport, params stepParams,
) (outcome, error) {
	proc, err := transport.NewProcess(ctx, params.Bin.Path, params.Bin.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to create proc: %w", err)
	}

	var startPayload struct {
		cmd string
	}
	startPayload.cmd = proc.String()

	if err := emitEvent(ctx, TestStartEvent, startPayload, target, r.ev); err != nil {
		return nil, fmt.Errorf("cannot emit event: %w", err)
	}

	// try to start the process, if that succeeds then the outcome is the result of
	// waiting on the process for its result; this way there's a semantic difference
	// between "an error occured while launching" and "this was the outcome of the execution"
	out := proc.Start(ctx)
	if out == nil {
		out = proc.Wait(ctx)
	}

	if err := emitEvent(ctx, TestEndEvent, nil, target, r.ev); err != nil {
		return nil, fmt.Errorf("cannot emit event: %w", err)
	}

	return out, nil
}

func (r *TargetRunner) Run(ctx xcontext.Context, target *target.Target) error {
	ctx.Infof("Executing on target %s", target)

	// limit the execution time if specified
	timeQuota := r.ts.Constraints.TimeQuota
	if timeQuota != 0 {
		var cancel xcontext.CancelFunc
		ctx, cancel = xcontext.WithTimeout(ctx, time.Duration(timeQuota))
		defer cancel()
	}

	pe := test.NewParamExpander(target)

	var params stepParams
	if err := pe.ExpandObject(r.ts.stepParams, &params); err != nil {
		return err
	}

	transport, err := transport.NewTransport(params.Transport.Proto, params.Transport.Options, pe)
	if err != nil {
		return fmt.Errorf("fail to create transport: %w", err)
	}

	if params.OCPOutput {
		out, err := r.runWithOCP(ctx, target, transport, params)

		// for any ambiguity, out is an error interface, but it encodes whether the process
		// was launched sucessfully and it resulted in a failure; err means the launch failed
		if out != nil {
			return out
		}
		return err
	}

	out, err := r.runAny(ctx, target, transport, params)
	if out != nil {
		return out
	}
	return err
}
