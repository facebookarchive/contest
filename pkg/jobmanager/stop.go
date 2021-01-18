// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"fmt"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/job"
)

func (jm *JobManager) stop(ev *api.Event) *api.EventResponse {
	msg := ev.Msg.(api.EventStopMsg)
	jobID := msg.JobID
	// CancelJob is asynchronous, it closes the Job's cancellation signal which
	// is propagated all the way down to the TestRunner. TestRunner  will wait
	// TestRunnerShutdownTimeout before flagging the test as timed out. JobRunner
	// will attempt to call Release on TargetManager and will wait up to
	// TargetManagerTimeout for Release to return.
	err := jm.CancelJob(jobID)
	if err != nil {
		log.Errorf("Cannot stop job: %v", err)
		return &api.EventResponse{Err: fmt.Errorf("could not stop job: %v", err)}
	}
	_ = jm.emitEvent(jobID, job.EventJobCancelling)
	return &api.EventResponse{
		JobID:     jobID,
		Requestor: ev.Msg.Requestor(),
		Err:       nil,
		Status: &job.Status{
			Name:      "UnknownJobName",
			State:     string(job.EventJobCancelling),
			StartTime: time.Now(),
		},
	}
}
