// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
)

func (jm *JobManager) list(ev *api.Event) *api.EventResponse {
	evResp := &api.EventResponse{
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
	}
	msg, ok := ev.Msg.(api.EventListMsg)
	if !ok {
		evResp.Err = fmt.Errorf("invaid argument type %T", ev.Msg)
		return evResp
	}
	var queryFields []storage.JobQueryField
	if len(msg.States) > 0 {
		queryFields = append(queryFields, storage.QueryJobStates(msg.States...))
	}
	var tags []string
	if jm.config.instanceTag != "" {
		tags = job.AddTags(tags, jm.config.instanceTag)
	}
	tags = job.AddTags(tags, msg.Tags...)
	if len(tags) > 0 {
		queryFields = append(queryFields, storage.QueryJobTags(tags...))
	}
	jobQuery, err := storage.BuildJobQuery(queryFields...)
	if err != nil {
		evResp.Err = fmt.Errorf("failed to build job query: %w", err)
		return evResp
	}
	res, err := jm.jobStorageManager.ListJobs(jobQuery)
	if err != nil {
		evResp.Err = fmt.Errorf("failed to list jobs: %w", err)
		return evResp
	}
	evResp.JobIDs = res
	return evResp
}
