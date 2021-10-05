// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/facebookincubator/contest/pkg/xcontext"
)

type LocalTransport struct{}

func NewLocalTransport() Transport {
	return &LocalTransport{}
}

func (lt *LocalTransport) Start(ctx xcontext.Context, bin string, args []string) (ExecProcess, error) {
	return startLocalExecProcess(ctx, bin, args)
}

type localExecProcess struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func startLocalExecProcess(ctx xcontext.Context, bin string, args []string) (ExecProcess, error) {
	if err := checkBinary(bin); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe")
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe")
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	return &localExecProcess{cmd, stdout, stderr}, nil
}

func (lp *localExecProcess) Wait(_ xcontext.Context) error {
	if err := lp.cmd.Wait(); err != nil {
		var e *exec.ExitError
		if errors.As(err, &e) {
			return fmt.Errorf("process exited with error: %w", err)
		}

		return fmt.Errorf("failed to wait on process: %w", err)
	}

	return nil
}

func (lp *localExecProcess) Stdout() io.Reader {
	return lp.stdout
}

func (lp *localExecProcess) Stderr() io.Reader {
	return lp.stderr
}
