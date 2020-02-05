// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"fmt"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"
)

var jobLog = logging.GetLogger("pkg/runner")

// JobRunner implements logic to run, cancel and stop Jobs
type JobRunner struct {
	// targetMap keeps the association between JobID and list of targets.
	// This might be requested from clients using the JobRunner instance
	targetMap map[types.JobID][]*target.Target
	// targetLock protects the access to targetMap
	targetLock *sync.RWMutex
}

// GetTargets returns a list of acquired targets for JobID
func (jr *JobRunner) GetTargets(jobID types.JobID) []*target.Target {
	jr.targetLock.RLock()
	defer jr.targetLock.RUnlock()
	if _, ok := jr.targetMap[jobID]; !ok {
		return nil
	}
	return jr.targetMap[jobID]
}

// Run implements the main job running logic. It holds a registry of all running
// jobs that can be referenced when when cancellation/pause/stop requests come in
//
// It returns:
//
// * [][]job.Report: all the run reports, grouped by run, sorted from first to
//                   last
// * []job.Report:   all the final reports
// * error:          an error, if any
func (jr *JobRunner) Run(j *job.Job) ([][]*job.Report, []*job.Report, error) {
	var (
		err                    error
		runReport, finalReport *job.Report
		run                    uint
		testResults            []*test.TestResult
	)

	if j.Runs == 0 {
		jobLog.Infof("Running job '%s' (id %v) indefinitely", j.Name, j.ID)
	} else {
		jobLog.Infof("Running job '%s' %d times", j.Name, j.Runs)
	}
	// TODO make this configurable
	lockTimeout := 10 * time.Second
	tl := inmemory.New(lockTimeout)
	ev := storage.NewTestEventFetcher()
	var (
		allRunsReports [][]*job.Report
		thisRunReports []*job.Report
	)
	for {
		if j.Runs != 0 && run == j.Runs {
			break
		}
		for idx, t := range j.Tests {
			if j.IsCancelled() {
				jobLog.Debugf("Cancellation requested, skipping test #%d of run #%d", idx, run+1)
				break
			}
			jobLog.Infof("Run #%d: fetching targets for test '%s'", run+1, t.Name)
			bundle := t.TargetManagerBundle
			var (
				targets   []*target.Target
				targetsCh = make(chan []*target.Target, 1)
				errCh     = make(chan error, 1)
			)
			go func() {
				// the Acquire semantic is synchronous, so that the implementation
				// is simpler on the user's side. We run it in a goroutine in
				// order to use a timeout for target acquisition.
				targets, err := bundle.TargetManager.Acquire(j.ID, j.CancelCh, bundle.AcquireParameters, tl)
				if err != nil {
					errCh <- err
					targetsCh <- nil
					return
				}
				if allAreLocked, _, notLocked := tl.CheckLocks(j.ID, targets); !allAreLocked {
					errCh <- fmt.Errorf("Could not lock %d targets out of %d are not locked: %v", len(notLocked), len(targets), notLocked)
					targetsCh <- nil
				}
				errCh <- nil
				targetsCh <- targets
			}()
			// wait for targets up to a certain amount of time
			select {
			case err := <-errCh:
				targets = <-targetsCh
				if err != nil {
					jobLog.Warningf("Run #%d: cannot fetch targets for test '%s': %v", run+1, t.Name, err)
					return nil, nil, err
				}
				// Associate the targets with the job for later retrievel
				jr.targetLock.Lock()
				jr.targetMap[j.ID] = targets
				jr.targetLock.Unlock()

			case <-time.After(config.TargetManagerTimeout):
				return nil, nil, fmt.Errorf("target manager acquire timed out after %s", config.TargetManagerTimeout)
			case <-j.CancelCh:
				jobLog.Infof("cancellation requested for job ID %v", j.ID)
				return nil, nil, nil
			}

			// refresh the target locks periodically, by extending their
			// expiration time. If the job is cancelled, the locks are released.
			// If the job is paused (e.g. because we are migrating the ConTest
			// instance or upgrading it), the locks are not released, because we
			// may want to resume once the new ConTest instance starts.
			done := make(chan struct{})
			go func(j *job.Job, tl target.Locker, targets []*target.Target, lockTimeout time.Duration) {
				for {
					select {
					case <-j.CancelCh:
						// unlock targets
						if err := tl.Unlock(j.ID, targets); err != nil {
							log.Warningf("Failed to unlock targets (%v) for job ID %d: %v", targets, j.ID, err)
						}
						return
					case <-j.PauseCh:
						// do not unlock targets, we can resume later, or let
						// them expire
						log.Debugf("Received pause request, NOT releasing targets so the job can be resumed")
						return
					case <-done:
						if err := tl.Unlock(j.ID, targets); err != nil {
							log.Warningf("Failed to unlock %d target(s) (%v): %v", len(targets), targets, err)
						}
						log.Infof("Unlocked %d target(s) for job ID %d", len(targets), j.ID)
						return
					case <-time.After(lockTimeout):
						// refresh the locks before the timeout expires
						if err := tl.RefreshLocks(j.ID, targets); err != nil {
							log.Warningf("Failed to refresh %d locks for job ID %d: %v", len(targets), j.ID, err)
						}
					}
				}
			}(j, tl, targets, lockTimeout)

			// Run the job
			jobLog.Infof("Run #%d: running test #%d for job '%s' (job ID: %d) on %d targets", run+1, idx, j.Name, j.ID, len(targets))
			runner := NewTestRunner()
			testResult, runErr := runner.Run(j.CancelCh, j.PauseCh, t, targets, j.ID)
			if testResult != nil {
				testResults = append(testResults, testResult)
			}
			// Job is done, release all the targets
			go func() {
				// the Release semantic is synchronous, so that the implementation
				// is simpler on the user's side. We run it in a goroutine in
				// order to use a timeout for target acquisition. If Release fails, whether
				// due to an error or for a timeout, the whole Job is considered failed
				errCh <- bundle.TargetManager.Release(j.ID, j.CancelCh, bundle.ReleaseParameters)
				// signal that we are done to the goroutine that refreshes the
				// locks.
				done <- struct{}{}
			}()
			select {
			case err := <-errCh:
				if err != nil {
					errRelease := fmt.Sprintf("Failed to release targets: %v", err)
					jobLog.Errorf(errRelease)
					return nil, nil, fmt.Errorf(errRelease)
				}
			case <-time.After(config.TargetManagerTimeout):
				return nil, nil, fmt.Errorf("target manager release timed out after %s", config.TargetManagerTimeout)
			case <-j.CancelCh:
				jobLog.Infof("cancellation requested for job ID %v", j.ID)
				return nil, nil, nil
			}
			// return the Run error only after releasing the targets, and only
			// if we are not running indefinitely.
			// TODO do the next runs even if one fails. We are interested in the
			// signal from all of them. Or not? Should this go behind a flag?
			if runErr != nil {
				return nil, nil, runErr
			}
			if len(testResults) == 0 {
				jobLog.Warningf("Skipping reporting phase because test did not produce any result")
				return nil, nil, fmt.Errorf("Report skipped because test did not produce any result")
			}
			thisRunReports = make([]*job.Report, 0)
			for _, bundle := range j.RunReporterBundles {
				runReport, err = bundle.Reporter.RunReport(j.CancelCh, bundle.Parameters, run+1, testResult, ev)
				if err != nil {
					jobLog.Warningf("Run reporter failed while calculating test results, proceeding anyway: %v", err)
				} else {
					if runReport.Success {
						jobLog.Printf("Run #%d of job %d considered successful", run+1, j.ID)
					} else {
						jobLog.Errorf("Run #%d of job %d considered failed", run+1, j.ID)
					}
				}
				// TODO run report must be sent to the storage layer as soon as it's
				//      ready, not at the end of the runs. This requires a change in
				//      how we store and expose reports, because this will require
				//      one DB entry per run report rather than one for all of them.
				thisRunReports = append(thisRunReports, runReport)
			}
		}
		allRunsReports = append(allRunsReports, thisRunReports)
		if j.IsCancelled() {
			jobLog.Debugf("Cancellation requested, skipping run #%d", run+1)
			break
		}
		// don't sleep on the last run
		if j.Runs == 0 || (j.Runs > 1 && run < j.Runs-1) {
			jobLog.Infof("Sleeping %s before the next run...", j.RunInterval)
			time.Sleep(j.RunInterval)
		}
		run++
	}
	// We completed the test runs, we can now calculate the final results of the
	// job, if any. The final reporters are always called, even if the job is
	// cancelled, so we can report with whatever we have collected so far.
	if j.IsCancelled() {
		return nil, nil, nil
	}
	var finalReports []*job.Report
	for _, bundle := range j.FinalReporterBundles {
		finalReport, err = bundle.Reporter.FinalReport(j.CancelCh, bundle.Parameters, testResults, ev)
		if err != nil {
			jobLog.Warningf("Final reporter failed while calculating test results, proceeding anyway: %v", err)
		} else {
			if finalReport.Success {
				jobLog.Printf("Job %d (%d runs out of %d desired) considered successful", j.ID, run, j.Runs)
			} else {
				jobLog.Errorf("Job %d (%d runs out of %d desired) considered failed", j.ID, run, j.Runs)
			}
		}
		finalReports = append(finalReports, finalReport)
	}

	return allRunsReports, finalReports, nil
}

// NewJobRunner returns a new JobRunner, which holds an empty registry of jobs
func NewJobRunner() *JobRunner {
	jr := JobRunner{}
	jr.targetMap = make(map[types.JobID][]*target.Target)
	jr.targetLock = &sync.RWMutex{}
	return &jr
}
