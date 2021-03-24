// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/job"
)

func (jm *JobManager) list(ev *api.Event) *api.EventResponse {
	ctx := ev.Context
	evResp := &api.EventResponse{
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
	}
	msg, ok := ev.Msg.(api.EventListMsg)
	if !ok {
		evResp.Err = fmt.Errorf("invaid argument type %T", ev.Msg)
		return evResp
	}
	jobQuery := msg.Query
	if jm.config.instanceTag != "" {
		jobQuery.Tags = job.AddTags(jobQuery.Tags, jm.config.instanceTag)
	}
	res, err := jm.jsm.ListJobs(ctx, jobQuery)
	if err != nil {
		evResp.Err = fmt.Errorf("failed to list jobs: %w", err)
		return evResp
	}
	evResp.JobIDs = res
	return evResp
}
