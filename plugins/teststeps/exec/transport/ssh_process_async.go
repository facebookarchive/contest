// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/insomniacslk/xjson"
	"github.com/kballard/go-shellquote"
	"golang.org/x/crypto/ssh"
)

type sshProcessAsync struct {
	addr         string
	clientConfig *ssh.ClientConfig
	cmd          string
	agent        string

	outWriter io.WriteCloser
	errWriter io.WriteCloser

	closeOnWait []io.Closer
	exitChan    chan error

	stack *deferedStack
}

func newSSHProcessAsync(
	ctx xcontext.Context,
	addr string, clientConfig *ssh.ClientConfig,
	agent string, timeQuota xjson.Duration,
	bin string, args []string,
	stack *deferedStack,
) (Process, error) {
	// build the command to run remotely
	agentArgs := []string{agent}
	if timeQuota != 0 {
		agentArgs = append(agentArgs, fmt.Sprintf("--time-quota=%s", timeQuota.String()))
	}
	agentArgs = append(agentArgs, "start", bin)
	agentArgs = append(agentArgs, args...)

	cmd := shellquote.Join(agentArgs...)
	exitChan := make(chan error, 1)

	return &sshProcessAsync{
		addr:         addr,
		clientConfig: clientConfig,
		cmd:          cmd,
		agent:        agent,
		closeOnWait:  []io.Closer{},
		exitChan:     exitChan,
		stack:        stack,
	}, nil
}

func (spa *sshProcessAsync) Start(ctx xcontext.Context) error {
	errChan := make(chan error, 1)
	resChan := make(chan string, 1)

	go func() {
		// NOTE: golang doesnt support forking, so the started process needs to be
		// forcefully detached by closing the ssh session; detach is defered here
		client, err := ssh.Dial("tcp", spa.addr, spa.clientConfig)
		if err != nil {
			errChan <- fmt.Errorf("cannot connect to SSH server %s: %v", spa.addr, err)
			return
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			errChan <- fmt.Errorf("cannot create SSH session to server: %v", err)
			return
		}
		defer session.Close()

		stdout, err := session.StdoutPipe()
		if err != nil {
			errChan <- fmt.Errorf("failed to get stdout pipe")
			return
		}

		ctx.Debugf("starting remote agent: %s", spa.cmd)
		if err := session.Start(spa.cmd); err != nil {
			errChan <- fmt.Errorf("failed to start process: %w", err)
			return
		}

		// read the session id that the agent will put on stdout
		s := bufio.NewScanner(stdout)
		if !s.Scan() {
			errChan <- fmt.Errorf("agent did not return a session id")
			return
		}
		resChan <- s.Text()
	}()

	select {
	case err := <-errChan:
		return err

	case sid := <-resChan:
		ctx.Debugf("remote sid: %s", sid)

		outWriter := spa.outWriter
		if outWriter == nil {
			var err error
			outWriter, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err != nil {
				return err
			}
		}

		errWriter := spa.errWriter
		if errWriter == nil {
			var err error
			errWriter, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err != nil {
				return err
			}
		}

		mon := &asyncMonitor{spa.addr, spa.clientConfig, spa.agent, sid}
		go mon.Start(ctx, outWriter, errWriter, spa.exitChan)
		return nil

	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout while starting agent")

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (spa *sshProcessAsync) Wait(_ xcontext.Context) error {
	defer spa.stack.Done()

	// wait for process
	err := <-spa.exitChan

	var e *ssh.ExitError
	if errors.As(err, &e) {
		return fmt.Errorf("process exited with error: %w", e)
	}

	if err != nil {
		return fmt.Errorf("failed to wait on process: %w", err)
	}
	return nil
}

func (spa *sshProcessAsync) StdoutPipe() (io.Reader, error) {
	r, w := io.Pipe()

	spa.outWriter = w
	spa.closeOnWait = append(spa.closeOnWait, r)
	return r, nil
}

func (spa *sshProcessAsync) StderrPipe() (io.Reader, error) {
	r, w := io.Pipe()

	spa.errWriter = w
	spa.closeOnWait = append(spa.closeOnWait, r)
	return r, nil
}

func (spa *sshProcessAsync) String() string {
	return spa.cmd
}

// TODO: maybe extract this to a package?
const (
	ProcessFinishedExitCode = 13
	DeadAgentExitCode       = 14
)

type asyncMonitor struct {
	addr         string
	clientConfig *ssh.ClientConfig

	agent string
	sid   string
}

func (m *asyncMonitor) Start(
	ctx xcontext.Context,
	outWriter io.WriteCloser, errWriter io.WriteCloser,
	exitChan chan<- error,
) {
	defer outWriter.Close()
	defer errWriter.Close()

	for {
		select {
		case <-time.After(time.Second):
			ctx.Debugf("polling remote process: %s", m.sid)

			stdout, stderr, err, runerr := m.runAgent(ctx, "poll")
			if err != nil {
				ctx.Warnf("failed to run agent: %w", err)
				continue
			}

			// append stdout, stderr; blocking until read
			if _, err := outWriter.Write(stdout); err != nil {
				ctx.Warnf("failed to write to stdout pipe: %w", err)
				continue
			}

			if _, err := errWriter.Write(stderr); err != nil {
				ctx.Warnf("failed to write to stderr pipe: %w", err)
				continue
			}

			if runerr != nil {
				var em *ssh.ExitMissingError
				if errors.As(runerr, &em) {
					if err := m.reap(ctx); err != nil {
						ctx.Warnf("monitor error: %w", err)
					}

					// process exited without an error or signal; this is a ssh server error
					exitChan <- fmt.Errorf("internal ssh server error: %w", em)
					return
				}

				var ee *ssh.ExitError
				if errors.As(runerr, &ee) {
					if err := m.reap(ctx); err != nil {
						ctx.Warnf("monitor error: %w", err)
					}

					switch ee.ExitStatus() {
					case ProcessFinishedExitCode:
						// agent controlled process exited by itself
						exitChan <- nil

					case DeadAgentExitCode:
						// agent killed itself due to time quota or other error
						exitChan <- fmt.Errorf("agent exceeded time quota or just crashed")

					default:
						exitChan <- ee
					}
					return
				}

				// process is done, but there's some other internal error
				exitChan <- runerr
			}

		case <-ctx.Done():
			ctx.Debugf("killing remote process, reason: cancellation")

			err := m.kill(ctx)
			if err := m.reap(ctx); err != nil {
				ctx.Warnf("monitor error: %w", err)
			}

			exitChan <- err
			return
		}
	}
}

func (m *asyncMonitor) kill(ctx xcontext.Context) error {
	ctx.Debugf("killing remote process: %s", m.sid)

	_, _, err, runerr := m.runAgent(ctx, "kill")
	if err != nil {
		return fmt.Errorf("failed to start agent kill: %w", err)
	}
	if runerr != nil {
		// note: this should never happen
		return fmt.Errorf("failed to kill remote process: %w", runerr)
	}

	return nil
}

func (m *asyncMonitor) reap(ctx xcontext.Context) error {
	ctx.Debugf("reaping remote process: %s", m.sid)

	_, _, err, runerr := m.runAgent(ctx, "wait")
	if err != nil {
		return fmt.Errorf("failed to start agent reap: %w", err)
	}
	if runerr != nil {
		return fmt.Errorf("failed to reap remote process: %w", runerr)
	}

	return nil
}

func (m *asyncMonitor) runAgent(ctx xcontext.Context, verb string) ([]byte, []byte, error, error) {
	client, err := ssh.Dial("tcp", m.addr, m.clientConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot connect to SSH server %s: %v", m.addr, err), nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot create SSH session to server: %w", err), nil
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	cmd := shellquote.Join(m.agent, verb, m.sid)
	ctx.Debugf("starting agent command: %s", cmd)
	if err := session.Start(cmd); err != nil {
		return nil, nil, fmt.Errorf("failed to start remote agent: %w", err), nil
	}

	// note: dont move this to the return line because stdout will be empty
	runerr := session.Wait()
	return stdout.Bytes(), stderr.Bytes(), nil, runerr
}
