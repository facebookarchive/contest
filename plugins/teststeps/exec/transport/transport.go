// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"encoding/json"
	"fmt"
	"io"

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

func NewTransport(proto string, config json.RawMessage) (Transport, error) {
	switch proto {
	case "local":
		return NewLocalTransport(config), nil

	default:
		return nil, fmt.Errorf("no such transport: %v", proto)
	}
}
