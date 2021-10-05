// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package types

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/facebookincubator/contest/pkg/xcontext"
)

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

type key string

const (
	KeyJobID = key("job_id")
	KeyRunID = key("run_id")
)

// JobIDFromContext is a helper to get the JobID, this is useful
// for plugins which need to know which job they are running.
// Not all context object everywhere have this set, but this is
// guaranteed to work in TargetManagers, TestSteps and Reporters
func JobIDFromContext(ctx xcontext.Context) (JobID, bool) {
	v, ok := ctx.Value(KeyJobID).(JobID)
	return v, ok
}

// RunIDFromContext is a helper to get the RunID.
// Not all context object everywhere have this set, but this is
// guaranteed to work in TargetManagers, TestSteps and RunReporters
func RunIDFromContext(ctx xcontext.Context) (RunID, bool) {
	v, ok := ctx.Value(KeyRunID).(RunID)
	return v, ok
}

// Duration is a JSON serializable time.Duration variant
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
		return nil

	case string:
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(parsed)
		return nil
	default:
		return errors.New("invalid duration")
	}
}

func (d Duration) IsZero() bool {
	return int64(d) == 0
}

func (d Duration) String() string {
	return time.Duration(d).String()
}
