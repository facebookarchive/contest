// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package types

import "strconv"

// JobID represents a unique job identifier
type JobID uint64

// RunID represents the id of a run within the Job
type RunID uint64

func (v JobID) String() string {
	return strconv.FormatUint(uint64(v), 10)
}

func (v RunID) String() string {
	return strconv.FormatUint(uint64(v), 10)
}
