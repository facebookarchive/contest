// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"github.com/facebookincubator/contest/pkg/job"
)

// JobEventEmitterFetcher wraps all the job-related emitters and fetchers.
type JobEventEmitterFetcher struct {
	JobRequestEmitterFetcher
	JobReportEmitterFetcher
}

// NewJobEventEmitterFetcher creates a new emitter/fetcher object for job events
func NewJobEventEmitterFetcher() job.EventEmitterFetcher {
	return JobEventEmitterFetcher{}
}
