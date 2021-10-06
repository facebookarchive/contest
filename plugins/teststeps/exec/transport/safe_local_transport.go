// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build !unsafe

package transport

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/xcontext"
)

type LocalTransport struct{}

func NewLocalTransport() Transport {
	return &LocalTransport{}
}

func (lt *LocalTransport) Start(ctx xcontext.Context, bin string, args []string) (ExecProcess, error) {
	return nil, fmt.Errorf("unavailable without unsafe build tag")
}
