// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strconv"
	"time"

	"github.com/kballard/go-shellquote"
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

	Timeout types.Duration `json:"timeout,omitempty"`
	// TODO: add transfer binary to remote option
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
	clientConfig := ssh.ClientConfig{
		User: st.User,
		Auth: auth,
		// TODO expose this in the plugin arguments
		//HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(st.Timeout),
	}

	// TODO: copy bin if needed
	// TODO: use agent if needed
	return startSSHExecProcess(ctx, bin, args, addr, clientConfig)
}

type sshExecProcess struct {
	client        *ssh.Client
	session       *ssh.Session
	keepAliveDone chan struct{}

	stdout io.Reader
	stderr io.Reader
}

func startSSHExecProcess(ctx xcontext.Context, bin string, args []string, addr string, clientConfig ssh.ClientConfig) (ExecProcess, error) {
	client, err := ssh.Dial("tcp", addr, &clientConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to SSH server %s: %v", addr, err)
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("cannot create SSH session to server %s: %v", addr, err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe")
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe")
	}

	cmd := shellquote.Join(append([]string{bin}, args...)...)
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

	return &sshExecProcess{client, session, keepAliveDone, stdout, stderr}, nil
}

func (sp *sshExecProcess) Wait(_ xcontext.Context) error {
	// close these no matter what error we get from the wait
	defer func() {
		close(sp.keepAliveDone)
	}()
	defer sp.client.Close()
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
