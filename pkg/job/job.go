// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"errors"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"

	"github.com/insomniacslk/xjson"
)

// Descriptor models the deserialized version of the JSON text given as
// input to the job creation request.
type Descriptor struct {
	JobName         string
	Tags            []string
	Runs            uint
	RunInterval     xjson.Duration
	TestDescriptors []test.Descriptor
	Reporting       Reporting
}

// Validate performs sanity checks on the job descriptor
func (d *Descriptor) Validate() error {

	if len(d.TestDescriptors) == 0 {
		return errors.New("need at least one TestDescriptor in the JobDescriptor")
	}
	if d.JobName == "" {
		return errors.New("job name cannot be empty")
	}
	if d.RunInterval < 0 {
		return errors.New("run interval must be non-negative")
	}

	if len(d.Reporting.RunReporters) == 0 && len(d.Reporting.FinalReporters) == 0 {
		return errors.New("at least one run reporter or one final reporter must be specified in a job")
	}
	for _, reporter := range d.Reporting.RunReporters {
		if strings.TrimSpace(reporter.Name) == "" {
			return errors.New("run reporters cannot have empty or all-whitespace names")
		}
	}
	return nil
}

// ExtendedDescriptor is a job descriptor which has been extended with the full
// description of the test and cleanup steps obtained from the test fetcher.
type ExtendedDescriptor struct {
	Descriptor
	StepsDescriptors []test.StepsDescriptors
}

// Job is used to run a type of test job on a given set of targets.
type Job struct {
	ID types.JobID

	// ExtendedDescriptor represents the descriptor that resulted in the
	// creation of this ConTest job.
	ExtendedDescriptor *ExtendedDescriptor

	Name string

	// a freeform list of strings that the user can provide to tag a job, and
	// subsequently use to search and aggregate.
	Tags []string

	// done is a job-wide channel that every stage should check to know
	// whether work should be stopped or not.
	Done chan struct{}

	// TODO: these channels should be owned by the JobManager
	// cancel is a job-wide channel used to request and detect job cancellation.
	CancelCh chan struct{}

	// pause is a job-wide channel used to request and detect job pausing.
	PauseCh chan struct{}

	// How many times a job has to run. 0 means infinite.
	// A "run" is the execution of a sequence of tests. For example, setting
	// Runs to 2 will execute all the tests defined in `Tests` once, and then
	// will execute them again.
	Runs uint

	// RunInterval is the interval between multiple runs, if more than one, or
	// unlimited, are specified.
	RunInterval time.Duration

	// Tests represents the compiled description of the tests
	Tests []*test.Test

	// RunReporterBundles and FinalReporterBundles wrap the reporter instances
	// chosen for the Job and its associated parameters, which have already
	// gone through validation
	RunReporterBundles   []*ReporterBundle
	FinalReporterBundles []*ReporterBundle
}

// Cancel closes the cancel channel to signal cancellation
func (j *Job) Cancel() {
	close(j.CancelCh)
}

// Pause closes the pause channel to signal pause
func (j *Job) Pause() {
	close(j.PauseCh)
}

// IsCancelled returns whether the job has been cancelled
func (j *Job) IsCancelled() bool {
	select {
	case _, ok := <-j.CancelCh:
		return !ok
	default:
		return false
	}
}

// InfoFetcher defines how to fetch job information
type InfoFetcher interface {
	FetchJob(types.JobID) (*Job, error)
	FetchJobs([]types.JobID) ([]*Job, error)
	FetchJobIDsByServerID(serverID string) ([]types.JobID, error)
}
