// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"context"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/types"
)

// Transport abstracts different ways of talking to contest server.
// This interface strictly only uses contest data structures.
type Transport interface {
	Version(ctx context.Context, requestor string) (*api.VersionResponse, error)
	Start(ctx context.Context, requestor string, jobDescriptor string) (*api.StartResponse, error)
	Stop(ctx context.Context, requestor string, jobID types.JobID) (*api.StopResponse, error)
	Status(ctx context.Context, requestor string, jobID types.JobID) (*api.StatusResponse, error)
	Retry(ctx context.Context, requestor string, jobID types.JobID) (*api.RetryResponse, error)
}
