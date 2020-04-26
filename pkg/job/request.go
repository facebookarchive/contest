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
	JobID         types.JobID
	JobName       string
	Requestor     string
	ServerID      string
	RequestTime   time.Time
	JobDescriptor string
	// TestDescriptors are the fetched test steps as per the test fetcher
	// defined in the JobDescriptor above.
	TestDescriptors string
}
