// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"github.com/facebookincubator/contest/pkg/types"
	"time"
)

// Request represents an incoming Job request which should be persisted in storage
type Request struct {
	JobID         types.JobID
	JobName       string
	Requestor     string
	RequestTime   time.Time
	JobDescriptor string
}

// RequestEmitter is an interface implemented by creator objects that
// create Request objects
type RequestEmitter interface {
	Emit(request *Request) (types.JobID, error)
}

// RequestFetcher is an interface implemented by fetcher objects that fetch
// job requests objects
type RequestFetcher interface {
	Fetch(id types.JobID) (*Request, error)
}

// RequestFetcher is an interface implemented by objects that implement both
// request creator and request fetcher interface
type RequestEmitterFetcher interface {
	RequestEmitter
	RequestFetcher
}
