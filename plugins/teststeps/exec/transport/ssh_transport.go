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
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kballard/go-shellquote"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

type SSHTransportConfig struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`

	User         string `json:"user,omitempty"`
	Password     string `json:"password,omitempty"`
	IdentityFile string `json:"identity_file,omitempty"`

	Timeout    types.Duration `json:"timeout,omitempty"`
	SendBinary bool           `json:"send_binary,omitempty"`
	AsyncAgent *string        `json:"async_agent,omitempty"`
}

func DefaultSSHTransportConfig() SSHTransportConfig {
	return SSHTransportConfig{
		Port:    22,
		Timeout: types.Duration(10 * time.Minute),
	}
}

type SSHTransport struct {
	SSHTransportConfig
}

func NewSSHTransport(config SSHTransportConfig) Transport {
	return &SSHTransport{config}
}

func (st *SSHTransport) Start(ctx xcontext.Context, bin string, args []string) (ExecProcess, error) {
	var signer ssh.Signer
	if st.IdentityFile != "" {
		key, err := ioutil.ReadFile(st.IdentityFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read private key at %s: %v", st.IdentityFile, err)
		}
		signer, err = ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("cannot parse private key: %v", err)
		}
	}

	auth := []ssh.AuthMethod{}
	if signer != nil {
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if st.Password != "" {
		auth = append(auth, ssh.Password(st.Password))
	}

	addr := net.JoinHostPort(st.Host, strconv.Itoa(st.Port))
	clientConfig := &ssh.ClientConfig{
		User: st.User,
		Auth: auth,
		// TODO expose this in the plugin arguments
		//HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(st.Timeout),
	}

	// stack mechanism similar to defer, but run after the exec process ends
	stack := newDeferedStack()

	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to SSH server %s: %v", addr, err)
	}

	// cleanup the ssh client after the operations have ended
	stack.Add(func() {
		if err := client.Close(); err != nil {
			ctx.Warnf("failed to close SSH client: %w", err)
		}
	})

	if st.SendBinary {
		if err := checkBinary(bin); err != nil {
			return nil, err
		}

		bin, err = st.sendFile(ctx, client, bin, 0500)
		if err != nil {
			return nil, fmt.Errorf("cannot send binary to remote ssh: %w", err)
		}

		// cleanup the sent file so we don't leave hanging files around
		stack.Add(func() {
			ctx.Debugf("cleaning remote file: %s", bin)
			if err := st.unlinkFile(ctx, client, bin); err != nil {
				ctx.Warnf("failed to cleanup remote file: %w", err)
			}
		})
	}

	if st.AsyncAgent != nil {
		return st.startAsync(ctx, client, addr, clientConfig, bin, args, stack)
	}
	return st.start(ctx, client, bin, args, stack)
}

func (st *SSHTransport) start(ctx xcontext.Context, client *ssh.Client, bin string, args []string, stack *deferedStack) (ExecProcess, error) {
	return startSSHExecProcess(ctx, client, bin, args, stack)
}

func (st *SSHTransport) startAsync(
	ctx xcontext.Context,
	client *ssh.Client, addr string, clientConfig *ssh.ClientConfig,
	bin string, args []string,
	stack *deferedStack,
) (ExecProcess, error) {
	// we always need the agent for the async case
	agent, err := st.sendFile(ctx, client, *st.AsyncAgent, 0500)
	if err != nil {
		return nil, fmt.Errorf("failed to send agent: %w", err)
	}

	stack.Add(func() {
		ctx.Debugf("cleaning async agent: %s", agent)
		if err := st.unlinkFile(ctx, client, agent); err != nil {
			ctx.Warnf("failed to cleanup asyng agent: %w", err)
		}
	})

	return startSSHExecProcessAsync(ctx, addr, clientConfig, agent, bin, args, stack)
}

func (st *SSHTransport) sendFile(ctx xcontext.Context, client *ssh.Client, bin string, mode os.FileMode) (string, error) {
	sftp, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create sftp client: %w", err)
	}
	defer sftp.Close()

	remoteBin := fmt.Sprintf("/tmp/exec_bin_%s", uuid.New().String())
	fout, err := sftp.Create(remoteBin)
	if err != nil {
		return "", fmt.Errorf("failed to create sftp file: %w", err)
	}
	defer fout.Close()

	fin, err := os.Open(bin)
	if err != nil {
		return "", fmt.Errorf("cannot open source bin file: %w", err)
	}
	defer fin.Close()

	ctx.Debugf("sending file to remote: %s", remoteBin)
	_, err = fout.ReadFrom(fin)
	if err != nil {
		return "", fmt.Errorf("failed to send file: %w", err)
	}

	return remoteBin, fout.Chmod(mode)
}

func (st *SSHTransport) unlinkFile(ctx xcontext.Context, client *ssh.Client, bin string) error {
	sftp, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create sftp client: %w", err)
	}
	defer sftp.Close()

	return sftp.Remove(bin)
}

type sshExecProcess struct {
	session       *ssh.Session
	keepAliveDone chan struct{}

	stdout io.Reader
	stderr io.Reader

	stack *deferedStack
}

func startSSHExecProcess(ctx xcontext.Context, client *ssh.Client, bin string, args []string, stack *deferedStack) (ExecProcess, error) {
	var stdin bytes.Buffer
	return startSSHExecProcessWithStdin(ctx, client, bin, args, &stdin, stack)
}

func startSSHExecProcessWithStdin(
	ctx xcontext.Context, client *ssh.Client,
	bin string, args []string,
	stdin io.Reader,
	stack *deferedStack,
) (ExecProcess, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("cannot create SSH session to server: %v", err)
	}

	// set fds for the remote process
	session.Stdin = stdin

	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe")
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe")
	}

	cmd := shellquote.Join(append([]string{bin}, args...)...)
	ctx.Debugf("starting remote binary: %s", cmd)

	if err := session.Start(cmd); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	// start a keepalive goro to keep sending to the ssh server
	keepAliveDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-keepAliveDone:
				return

			case <-time.After(5 * time.Second):
				ctx.Debugf("sending sigcont to ssh server...")
				err = session.Signal(ssh.Signal("CONT"))
				if err != nil {
					ctx.Warnf("failed to send CONT to ssh server: %w", err)
				}
			}
		}
	}()

	return &sshExecProcess{session, keepAliveDone, stdout, stderr, stack}, nil
}

func (sp *sshExecProcess) Wait(_ xcontext.Context) error {
	// close these no matter what error we get from the wait
	defer func() {
		sp.stack.Done()
		close(sp.keepAliveDone)
	}()
	defer sp.session.Close()

	if err := sp.session.Wait(); err != nil {
		return fmt.Errorf("failed to wait on process: %w", err)
	}

	return nil
}

func (sp *sshExecProcess) Stdout() io.Reader {
	return sp.stdout
}

func (sp *sshExecProcess) Stderr() io.Reader {
	return sp.stderr
}

type sshExecProcessAsync struct {
	stdout *io.PipeReader
	stderr *io.PipeReader

	alive chan struct{}
	stack *deferedStack
}

func startSSHExecProcessAsync(
	ctx xcontext.Context,
	addr string, clientConfig *ssh.ClientConfig,
	agent string,
	bin string, args []string,
	stack *deferedStack,
) (ExecProcess, error) {
	errChan := make(chan error, 1)
	resChan := make(chan string, 1)

	go func() {
		client, err := ssh.Dial("tcp", addr, clientConfig)
		if err != nil {
			errChan <- fmt.Errorf("cannot connect to SSH server %s: %v", addr, err)
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

		// TODO: pass a max time to live to the agent
		cmd := shellquote.Join(append([]string{agent, "start", bin}, args...)...)
		ctx.Debugf("starting remote agent: %s", cmd)

		// NOTE: golang doesnt support forking, so the started process needs to be
		// forcefully detached by closing the ssh session
		if err := session.Start(cmd); err != nil {
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
		return nil, err

	case sid := <-resChan:
		ctx.Debugf("remote sid: %s", sid)

		outReader, outWriter := io.Pipe()
		errReader, errWriter := io.Pipe()
		alive := make(chan struct{})

		mon := &asyncMonitor{addr, clientConfig, agent, sid}
		go mon.Start(ctx, outWriter, errWriter, alive)

		return &sshExecProcessAsync{outReader, errReader, alive, stack}, nil

	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout while starting agent")
	}
}

// TODO: maybe extract this to a package?
const (
	DeadProcessExitCode = 13
)

type asyncMonitor struct {
	addr         string
	clientConfig *ssh.ClientConfig

	agent string
	sid   string
}

func (m *asyncMonitor) Start(
	ctx xcontext.Context,
	outWriter *io.PipeWriter, errWriter *io.PipeWriter,
	alive chan<- struct{},
) {
	defer outWriter.Close()
	defer errWriter.Close()

	// TODO: add timeout cancel
	for {
		ctx.Debugf("polling remote process: %s", m.sid)
		stop, err := func() (bool, error) {
			stdout, stderr, runerr := m.runAgent("poll")

			// append stdout, stderr; blocking until read
			if _, err := outWriter.Write(stdout); err != nil {
				return false, fmt.Errorf("failed to write to stdout pipe: %w", err)
			}

			if _, err := errWriter.Write(stderr); err != nil {
				return false, fmt.Errorf("failed to write to stderr pipe: %w", err)
			}

			if runerr != nil {
				var e *ssh.ExitError
				if errors.As(runerr, &e) && e.ExitStatus() == DeadProcessExitCode {
					// the controlled process is no longer alive, signal
					close(alive)
					return true, m.reap(ctx)
				}

				return true, fmt.Errorf("process exited with error: %w", runerr)
			}

			return false, nil
		}()

		if err != nil {
			ctx.Warnf("monitor error: %w", err)
		}
		if stop {
			break
		}

		time.Sleep(time.Second)
	}
}

func (m *asyncMonitor) reap(ctx xcontext.Context) error {
	ctx.Debugf("reaping remote process: %s", m.sid)

	_, _, err := m.runAgent("wait")
	if err != nil {
		return fmt.Errorf("failed to reap remote process: %w", err)
	}

	return nil
}

func (m *asyncMonitor) runAgent(verb string) ([]byte, []byte, error) {
	client, err := ssh.Dial("tcp", m.addr, m.clientConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot connect to SSH server %s: %v", m.addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot create SSH session to server: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	cmd := shellquote.Join(m.agent, verb, m.sid)
	if err := session.Start(cmd); err != nil {
		return nil, nil, fmt.Errorf("failed to start remote agent: %w", err)
	}

	// wait for any exit code, the process already started
	runerr := session.Wait()
	return stdout.Bytes(), stderr.Bytes(), runerr
}

func (spa *sshExecProcessAsync) Wait(_ xcontext.Context) error {
	defer spa.stack.Done()

	<-spa.alive
	return nil
}

func (spa *sshExecProcessAsync) Stdout() io.Reader {
	return spa.stdout
}

func (spa *sshExecProcessAsync) Stderr() io.Reader {
	return spa.stderr
}

type deferedStack struct {
	funcs []func()

	closed bool
	done   chan struct{}

	mu sync.Mutex
}

func newDeferedStack() *deferedStack {
	s := &deferedStack{nil, false, make(chan struct{}), sync.Mutex{}}

	go func() {
		<-s.done
		for i := len(s.funcs) - 1; i >= 0; i-- {
			s.funcs[i]()
		}
	}()

	return s
}

func (s *deferedStack) Add(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.funcs = append(s.funcs, f)
}

func (s *deferedStack) Done() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	close(s.done)
	s.closed = true
}
