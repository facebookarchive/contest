// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package api

import (
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Listener defines the interface for an API listener. This is used to
// implement different API transports, like thrift, or gRPC.
type Listener interface {
	// Serve is responsible for starting the listener and calling the API
	// methods to communicate with the JobManager.
	// The channel is used for cancellation, which can be called by the
	// JobManager and should be handled by the listener to do a graceful
	// shutdown.
	Serve(xcontext.Context, *API) error
}
