// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strconv"
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

	// cleanup mechanism similar to defer, but run after the exec process ends
	done := make(chan struct{})
	var defered []func()

	go func() {
		<-done
		for i := len(defered) - 1; i >= 0; i-- {
			defered[i]()
		}
	}()

	client, err := ssh.Dial("tcp", addr, &clientConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to SSH server %s: %v", addr, err)
	}

	// cleanup the ssh client after the operations have ended
	defered = append(defered, func() {
		if err := client.Close(); err != nil {
			ctx.Warnf("failed to close SSH client: %w", err)
		}
	})

	// simple case, binary aready exists on the target
	if !st.SendBinary {
		return startSSHExecProcess(ctx, client, bin, args, done)
	}

	// TODO: use agent if needed
	remoteBin, err := st.sendFile(ctx, client, bin, 0500)
	if err != nil {
		return nil, fmt.Errorf("cannot send binary to remote ssh: %w", err)
	}

	// cleanup the sent file so we don't leave hanging files around
	defered = append(defered, func() {
		ctx.Debugf("cleaning remote file: %s", remoteBin)
		if err := st.unlinkFile(ctx, client, remoteBin); err != nil {
			ctx.Warnf("failed to cleanup remote file: %w", err)
		}
	})

	return startSSHExecProcess(ctx, client, remoteBin, args, done)
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

	done chan<- struct{}
}

func startSSHExecProcess(ctx xcontext.Context, client *ssh.Client, bin string, args []string, done chan<- struct{}) (ExecProcess, error) {
	var stdin bytes.Buffer
	return startSSHExecProcessWithStdin(ctx, client, bin, args, &stdin, done)
}

func startSSHExecProcessWithStdin(
	ctx xcontext.Context, client *ssh.Client,
	bin string, args []string,
	stdin io.Reader,
	done chan<- struct{},
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

	return &sshExecProcess{session, keepAliveDone, stdout, stderr, done}, nil
}

func (sp *sshExecProcess) Wait(_ xcontext.Context) error {
	// close these no matter what error we get from the wait
	defer func() {
		close(sp.done)
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
