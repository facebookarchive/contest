// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

type Transport interface {
	Start(ctx xcontext.Context, bin string, args []string) (ExecProcess, error)
}

type ExecProcess interface {
	Wait(ctx xcontext.Context) error

	Stdout() io.Reader
	Stderr() io.Reader
}

func NewTransport(proto string, configSource json.RawMessage, expander *test.ParamExpander) (Transport, error) {
	switch proto {
	case "local":
		return NewLocalTransport(), nil

	case "ssh":
		configTempl := DefaultSSHTransportConfig()
		if err := json.Unmarshal(configSource, &configTempl); err != nil {
			return nil, fmt.Errorf("unable to deserialize transport options: %w", err)
		}

		var config SSHTransportConfig
		if err := expander.ExpandObject(configTempl, &config); err != nil {
			return nil, err
		}

		return NewSSHTransport(config), nil

	default:
		return nil, fmt.Errorf("no such transport: %v", proto)
	}
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
