// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package exec

import (
	"encoding/json"
	"fmt"
	"io"
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

func consume(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		if _, err := r.Read(buf); err == io.EOF {
			break
		}
	}
}

func (r *TargetRunner) runWithOCP(
	ctx xcontext.Context, target *target.Target,
	transport transport.Transport, params stepParams,
) (outcome, error) {
	proc, err := transport.Start(ctx, params.Bin.Path, params.Bin.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to start proc: %w", err)
	}

	// TODO: refactor this to only get pipes on demand
	go consume(proc.Stderr())

	p := NewOCPEventParser(target, r.ev)
	dec := json.NewDecoder(proc.Stdout())
	for dec.More() {
		var root *OCPRoot
		dec.Decode(&root)

		p.Parse(ctx, root)
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
	proc, err := transport.Start(ctx, params.Bin.Path, params.Bin.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to start proc: %w", err)
	}

	go consume(proc.Stdout())
	go consume(proc.Stderr())

	// NOTE: these events technically aren't needed, but kept for symmetry with the ocp case
	if err := emitEvent(ctx, TestStartEvent, nil, target, r.ev); err != nil {
		return nil, fmt.Errorf("cannot emit event: %w", err)
	}
	out := proc.Wait(ctx)

	if err := emitEvent(ctx, TestEndEvent, nil, target, r.ev); err != nil {
		return nil, fmt.Errorf("cannot emit event: %w", err)
	}

	return out, nil
}

func (r *TargetRunner) Run(ctx xcontext.Context, target *target.Target) error {
	ctx.Infof("Executing on target %s", target)

	// limit the execution time if specified
	timeQuota := r.ts.Constraints.TimeQuota
	if !timeQuota.IsZero() {
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
