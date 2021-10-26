// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"time"

	"github.com/facebookincubator/contest/pkg/types"
)

// Request represents an incoming Job request which should be persisted in storage
type Request struct {
	JobID       types.JobID
	JobName     string
	Requestor   string
	ServerID    string
	RequestTime time.Time

	// JobDescriptor represents the raw descriptor as submitted by the user.
	JobDescriptor string

	// ExtendedDescriptor represents the deserialized job description extended
	// with with the full description of the test steps obtained from the test
	// fetcher. Since the descriptions of the test steps might not be inlined in
	// the descriptor itself, upon receiving a job request, job manager will fetch
	// the desciption and build extended descriptor accordingly. The extended
	// descriptor is then used upon requesting a resume without having a dependency
	// on the test fetcher.
	ExtendedDescriptor *ExtendedDescriptor
}
