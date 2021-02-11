// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"time"

	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"

	"github.com/insomniacslk/xjson"
)

// JobDescriptor models the JSON encoded blob which is given as input to the
// job creation request. A JobDescriptor embeds a list of TestDescriptor.
type JobDescriptor struct {
	JobName         string
	Tags            []string
	Runs            uint
	RunInterval     xjson.Duration
	TestDescriptors []*test.TestDescriptor
	Reporting       Reporting
	TargetManagerAcquireTimeout *xjson.Duration // optional
	TargetManagerReleaseTimeout *xjson.Duration // optional
}

// Job is used to run a type of test job on a given set of targets.
type Job struct {
	ID   types.JobID
	Name string
	// a freeform list of strings that the user can provide to tag a job, and
	// subsequently use to search and aggregate.
	Tags []string

	// TODO: StateCtx should be owned by the JobManager
	// cancel or pause is a job-wide channel used to request and detect job's state change.
	StateCtx       statectx.Context
	StateCtxPause  func()
	StateCtxCancel func()

	// How many times a job has to run. 0 means infinite.
	// A "run" is the execution of a sequence of tests. For example, setting
	// Runs to 2 will execute all the tests defined in `Tests` once, and then
	// will execute them again.
	Runs uint

	// RunInterval is the interval between multiple runs, if more than one, or
	// unlimited, are specified.
	RunInterval time.Duration

	// TargetManagerAcquireTimeout represents the maximum time that JobManager should wait for the execution of the Acquire function from the chosen TargetManager.
	TargetManagerAcquireTimeout time.Duration

	// TargetManagerReleaseTimeout represents the maximum time that JobManager should wait for the execution of the Release function from the chosen TargetManager.
	TargetManagerReleaseTimeout time.Duration

	// TestDescriptors is the string form of the fetched test step
	// descriptors.
	TestDescriptors string
	// Tests is the parsed form of the above TestDescriptors
	Tests []*test.Test
	// RunReporterBundles and FinalReporterBundles wrap the reporter instances
	// chosen for the Job and its associated parameters, which have already
	// gone through validation
	RunReporterBundles   []*ReporterBundle
	FinalReporterBundles []*ReporterBundle
}

// Cancel closes the cancel channel to signal cancellation
func (j *Job) Cancel() {
	j.StateCtxCancel()
}

// Pause closes the pause channel to signal pause
func (j *Job) Pause() {
	j.StateCtxPause()
}

// IsCancelled returns whether the job has been cancelled
func (j *Job) IsCancelled() bool {
	return j.StateCtx.Err() == statectx.ErrCanceled
}

func (j *Job) IsPaused() bool {
	return j.StateCtx.PausedCtx().Err() == statectx.ErrPaused
}

// InfoFetcher defines how to fetch job information
type InfoFetcher interface {
	FetchJob(types.JobID) (*Job, error)
	FetchJobs([]types.JobID) ([]*Job, error)
	FetchJobIDsByServerID(serverID string) ([]types.JobID, error)
}
