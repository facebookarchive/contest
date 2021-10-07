// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
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

	Async *struct {
		Agent     string         `json:"agent,omitempty"`
		TimeQuota types.Duration `json:"time_quota,omitempty"`
	} `json:"async,omitempty"`
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

func (st *SSHTransport) NewProcess(ctx xcontext.Context, bin string, args []string) (Process, error) {
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

	if st.Async != nil {
		return st.newAsync(ctx, client, addr, clientConfig, bin, args, stack)
	}
	return st.new(ctx, client, bin, args, stack)
}

func (st *SSHTransport) new(ctx xcontext.Context, client *ssh.Client, bin string, args []string, stack *deferedStack) (Process, error) {
	return newSSHProcess(ctx, client, bin, args, stack)
}

func (st *SSHTransport) newAsync(
	ctx xcontext.Context,
	client *ssh.Client, addr string, clientConfig *ssh.ClientConfig,
	bin string, args []string,
	stack *deferedStack,
) (Process, error) {
	// we always need the agent for the async case
	agent, err := st.sendFile(ctx, client, st.Async.Agent, 0500)
	if err != nil {
		return nil, fmt.Errorf("failed to send agent: %w", err)
	}

	stack.Add(func() {
		ctx.Debugf("cleaning async agent: %s", agent)
		if err := st.unlinkFile(ctx, client, agent); err != nil {
			ctx.Warnf("failed to cleanup asyng agent: %w", err)
		}
	})

	return newSSHProcessAsync(ctx, addr, clientConfig, agent, st.Async.TimeQuota, bin, args, stack)
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
