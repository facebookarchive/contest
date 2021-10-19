// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/kballard/go-shellquote"
	"golang.org/x/crypto/ssh"
)

type sshProcess struct {
	session       *ssh.Session
	cmd           string
	keepAliveDone chan struct{}

	stack *deferedStack
}

func newSSHProcess(ctx xcontext.Context, client *ssh.Client, bin string, args []string, stack *deferedStack) (Process, error) {
	var stdin bytes.Buffer
	return newSSHProcessWithStdin(ctx, client, bin, args, &stdin, stack)
}

func newSSHProcessWithStdin(
	ctx xcontext.Context, client *ssh.Client,
	bin string, args []string,
	stdin io.Reader,
	stack *deferedStack,
) (Process, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("cannot create SSH session to server: %v", err)
	}

	// set fds for the remote process
	session.Stdin = stdin

	cmd := shellquote.Join(append([]string{bin}, args...)...)
	keepAliveDone := make(chan struct{})

	return &sshProcess{session, cmd, keepAliveDone, stack}, nil
}

func (sp *sshProcess) Start(ctx xcontext.Context) error {
	// important note: turns out that with some sshd configs/implementations
	// sending signals thru the ssh channel doesn't work (either not implemented or
	// refused due to privilege separation).
	// So to work around that, allocate a pty for this session and rely on SIGHUP
	// to kill the process remotely if the ctx gets cancelled.
	// This obviously has the limitation that the spawned process can just ignore
	// SIGHUP and control its own lifetime, but there's no other way to have this.
	if err := sp.session.RequestPty("xterm", 80, 120, ssh.TerminalModes{}); err != nil {
		return err
	}

	ctx.Debugf("starting remote binary: %s", sp.cmd)
	if err := sp.session.Start(sp.cmd); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	go func() {
		for {
			select {
			case <-sp.keepAliveDone:
				return

			case <-time.After(5 * time.Second):
				ctx.Debugf("sending sigcont to ssh server...")
				if err := sp.session.Signal(ssh.Signal("CONT")); err != nil {
					ctx.Warnf("failed to send CONT to ssh server: %w", err)
				}

			case <-ctx.Done():
				ctx.Debugf("killing ssh session because of cancellation...")

				// Part 2 of the cancellation. Normally this would send a SIGKILL here
				// but this is sometimes ignored by sshd based on config/impl, so instead
				// rely on SIGHUP to kill the process by just closing the session.
				// if err := sp.session.Signal(ssh.SIGINT); err != nil {
				// 	ctx.Warnf("failed to send KILL on context cancel: %w", err)
				// }

				sp.session.Close()
				return
			}
		}
	}()

	return nil
}

func (sp *sshProcess) Wait(c xcontext.Context) error {
	// close these no matter what error we get from the wait
	defer func() {
		sp.stack.Done()
		close(sp.keepAliveDone)
	}()
	defer sp.session.Close()

	if err := sp.session.Wait(); err != nil {
		var e *ssh.ExitError
		if errors.As(err, &e) {
			return fmt.Errorf("process exited with error: %w", e)
		}

		return fmt.Errorf("failed to wait on process: %w", err)
	}

	return nil
}

func (sp *sshProcess) StdoutPipe() (io.Reader, error) {
	stdout, err := sp.session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe")
	}

	return stdout, nil
}

func (sp *sshProcess) StderrPipe() (io.Reader, error) {
	stderr, err := sp.session.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe")
	}

	return stderr, nil
}

func (sp *sshProcess) String() string {
	return sp.cmd
}
