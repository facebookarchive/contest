// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package event

import (
	"time"

	"github.com/facebookincubator/contest/pkg/types"
)

// Query wraps information that are used to build event queries for all type of event objects
type Query struct {
	JobID            types.JobID
	EventNames       []Name
	EmittedStartTime time.Time
	EmittedEndTime   time.Time
}
