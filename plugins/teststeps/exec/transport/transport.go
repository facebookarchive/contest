// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

type Transport interface {
	NewProcess(ctx xcontext.Context, bin string, args []string) (Process, error)
}

type Process interface {
	Start(ctx xcontext.Context) error
	Wait(ctx xcontext.Context) error

	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)

	String() string
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
