// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/xcontext"

	"github.com/facebookincubator/contest/plugins/teststeps/exec/transport"
)

type outcome error

type ExpandedParams struct {
	bin       string
	args      []string
	transport transport.Transport
	ocpOutput bool
}

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

func (r *TargetRunner) expandParams(target *target.Target) (*ExpandedParams, error) {
	var err error
	params := &ExpandedParams{
		ocpOutput: r.ts.ocpOutput,
	}

	params.bin, err = r.ts.bin.Expand(target)
	if err != nil {
		return nil, fmt.Errorf("cannot expand binary path: %w", err)
	}

	for _, argParam := range r.ts.args {
		arg, err := argParam.Expand(target)
		if err != nil {
			return nil, fmt.Errorf("cannot expand command argument '%s': %v", argParam, err)
		}
		params.args = append(params.args, arg)
	}

	// check the now expanded binary path
	if err := checkBinary(params.bin); err != nil {
		return nil, err
	}

	params.transport = r.ts.transport
	params.ocpOutput = r.ts.ocpOutput
	return params, nil
}

func (r *TargetRunner) runWithOCP(ctx xcontext.Context, target *target.Target, params *ExpandedParams) (outcome, error) {
	proc, err := params.transport.Start(ctx, params.bin, params.args)
	if err != nil {
		return nil, fmt.Errorf("failed to start proc: %w", err)
	}

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

func (r *TargetRunner) runAny(ctx xcontext.Context, target *target.Target, params *ExpandedParams) (outcome, error) {
	proc, err := params.transport.Start(ctx, params.bin, params.args)
	if err != nil {
		return nil, fmt.Errorf("failed to start proc: %w", err)
	}

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

	params, err := r.expandParams(target)
	if err != nil {
		return err
	}

	if params.ocpOutput {
		out, err := r.runWithOCP(ctx, target, params)
		if out != nil {
			return out
		}
		return err
	}

	out, err := r.runAny(ctx, target, params)
	if out != nil {
		return out
	}
	return err
}

func canExecute(fi os.FileInfo) bool {
	// TODO: deal with acls?
	stat := fi.Sys().(*syscall.Stat_t)
	if stat.Uid == uint32(os.Getuid()) {
		return stat.Mode&0500 == 0500
	}

	if stat.Gid == uint32(os.Getgid()) {
		return stat.Mode&0050 == 0050
	}

	return stat.Mode&0005 == 0005
}

func checkBinary(bin string) error {
	// check binary exists and is executable
	fi, err := os.Stat(bin)
	if err != nil {
		return fmt.Errorf("no such file")
	}

	if !fi.Mode().IsRegular() {
		return fmt.Errorf("not a file")
	}

	if !canExecute(fi) {
		return fmt.Errorf("provided binary is not executable")
	}
	return nil
}
