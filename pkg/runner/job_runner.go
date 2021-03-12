// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// JobRunner implements logic to run, cancel and stop Jobs
type JobRunner struct {
	// targetMap keeps the association between JobID and list of targets.
	// This might be requested from clients using the JobRunner instance
	targetMap map[types.JobID][]*target.Target
	// targetLock protects the access to targetMap
	targetLock *sync.RWMutex
	// frameworkEventManager is used by the JobRunner to emit framework events
	frameworkEventManager frameworkevent.EmitterFetcher
	// testEvManager is used by the JobRunner to emit test events
	testEvManager testevent.Fetcher
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
	var run uint

	if j.Runs == 0 {
		j.StateCtx.Logger().Infof("Running job '%s' (id %v) indefinitely", j.Name, j.ID)
	} else {
		j.StateCtx.Logger().Infof("Running job '%s' %d times", j.Name, j.Runs)
	}
	tl := target.GetLocker()
	ev := storage.NewTestEventFetcher()

	var (
		runReports      []*job.Report
		allRunReports   [][]*job.Report
		allFinalReports []*job.Report
		runErr          error
	)

	for {
		if j.Runs != 0 && run == j.Runs {
			break
		}

		// If we can't emit the run start event, we ignore the error. The framework will
		// try to rebuild the status if it detects that an event might have gone missing
		payload := RunStartedPayload{RunID: types.RunID(run + 1)}
		err := jr.emitEvent(j.StateCtx, j.ID, EventRunStarted, payload)
		if err != nil {
			j.StateCtx.Logger().Warnf("Could not emit event run (run %d) start for job %d: %v", run+1, j.ID, err)
		}

		for idx, t := range j.Tests {
			if j.IsCancelled() {
				j.StateCtx.Logger().Debugf("Cancellation requested, skipping test #%d of run #%d", idx, run+1)
				break
			}
			j.StateCtx.Logger().Infof("Run #%d: fetching targets for test '%s'", run+1, t.Name)
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
				targets, err := bundle.TargetManager.Acquire(j.StateCtx, j.ID, j.TargetManagerAcquireTimeout+config.LockRefreshTimeout, bundle.AcquireParameters, tl)
				if err != nil {
					errCh <- err
					targetsCh <- nil
					return
				}
				// Lock all the targets returned by Acquire.
				// Targets can also be locked in the `Acquire` method, for
				// example to allow dynamic acquisition.
				// We lock them again to ensure that all the acquired
				// targets are locked before running the job.
				// Locking an already-locked target (by the same owner)
				// extends the locking deadline.
				if err := tl.Lock(j.StateCtx, j.ID, j.TargetManagerAcquireTimeout, targets); err != nil {
					errCh <- fmt.Errorf("Target locking failed: %w", err)
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
					err = fmt.Errorf("run #%d: cannot fetch targets for test '%s': %v", run+1, t.Name, err)
					j.StateCtx.Logger().Errorf(err.Error())
					return nil, nil, err
				}
				// Associate the targets with the job for later retrievel
				jr.targetLock.Lock()
				jr.targetMap[j.ID] = targets
				jr.targetLock.Unlock()

			case <-time.After(j.TargetManagerAcquireTimeout):
				return nil, nil, fmt.Errorf("target manager acquire timed out after %s", j.TargetManagerAcquireTimeout)
			case <-j.StateCtx.Done():
				j.StateCtx.Logger().Infof("cancellation requested for job ID %v", j.ID)
				return nil, nil, nil
			}

			// refresh the target locks periodically, by extending their
			// expiration time. If the job is cancelled, the locks are released.
			// If the job is paused (e.g. because we are migrating the ConTest
			// instance or upgrading it), the locks are not released, because we
			// may want to resume once the new ConTest instance starts.
			done := make(chan struct{})
			go func(j *job.Job, tl target.Locker, targets []*target.Target, refreshInterval time.Duration) {
				for {
					select {
					case <-j.StateCtx.Done():
						if err := tl.Unlock(j.StateCtx, j.ID, targets); err != nil {
							j.StateCtx.Logger().Warnf("Failed to unlock targets (%v) for job ID %d: %v", targets, j.ID, err)
						}
					case <-j.StateCtx.WaitFor(xcontext.Paused):
						j.StateCtx.Logger().Debugf("Received pause request, NOT releasing targets so the job can be resumed")
						return
					case <-done:
						if err := tl.Unlock(j.StateCtx, j.ID, targets); err != nil {
							j.StateCtx.Logger().Warnf("Failed to unlock %d target(s) (%v): %v", len(targets), targets, err)
						}
						j.StateCtx.Logger().Infof("Unlocked %d target(s) for job ID %d", len(targets), j.ID)
						return
					case <-time.After(refreshInterval):
						// refresh the locks before the timeout expires
						if err := tl.RefreshLocks(j.StateCtx, j.ID, targets); err != nil {
							j.StateCtx.Logger().Warnf("Failed to refresh %d locks for job ID %d: %v", len(targets), j.ID, err)
						}
					}
				}
				// refresh locks a bit faster than locking timeout to avoid races
			}(j, tl, targets, config.LockRefreshTimeout/10*9)

			// Emit events tracking targets acquisition
			header := testevent.Header{JobID: j.ID, RunID: types.RunID(run + 1), TestName: t.Name}
			testEventEmitter := storage.NewTestEventEmitter(header)

			if runErr = jr.emitAcquiredTargets(j.StateCtx, testEventEmitter, targets); runErr == nil {
				j.StateCtx.Logger().Infof("Run #%d: running test #%d for job '%s' (job ID: %d) on %d targets", run+1, idx, j.Name, j.ID, len(targets))
				testRunner := NewTestRunner()
				resumeState, err := testRunner.Run(j.StateCtx, t, targets, j.ID, types.RunID(run+1), nil)
				if err == xcontext.Paused {
					j.StateCtx.Logger().Debugf("Runner paused, state: %s", string(resumeState))
					// TODO(rojer): Persist the state.
				} else {
					runErr = err
				}
			}

			// Job is done, release all the targets
			go func() {
				// the Release semantic is synchronous, so that the implementation
				// is simpler on the user's side. We run it in a goroutine in
				// order to use a timeout for target acquisition. If Release fails, whether
				// due to an error or for a timeout, the whole Job is considered failed
				errCh <- bundle.TargetManager.Release(j.StateCtx, j.ID, bundle.ReleaseParameters)
				// signal that we are done to the goroutine that refreshes the
				// locks.
				close(done)
			}()
			select {
			case err := <-errCh:
				if err != nil {
					errRelease := fmt.Sprintf("Failed to release targets: %v", err)
					j.StateCtx.Logger().Errorf(errRelease)
					return nil, nil, fmt.Errorf(errRelease)
				}
			case <-time.After(j.TargetManagerReleaseTimeout):
				return nil, nil, fmt.Errorf("target manager release timed out after %s", j.TargetManagerReleaseTimeout)
			case <-j.StateCtx.Done():
				j.StateCtx.Logger().Infof("cancellation requested for job ID %v", j.ID)
				return nil, nil, nil
			}
			// return the Run error only after releasing the targets, and only
			// if we are not running indefinitely. An error returned by the TestRunner
			// is considered a fatal condition and will cause the termination of the
			// whole job.
			if runErr != nil {
				return nil, nil, runErr
			}
		}

		// Calculate results for this run via the registered run reporters reporters
		runCoordinates := job.RunCoordinates{JobID: j.ID, RunID: types.RunID(run + 1)}

		runReports = make([]*job.Report, 0, len(j.RunReporterBundles))
		for _, bundle := range j.RunReporterBundles {
			runStatus, err := jr.BuildRunStatus(runCoordinates, j)
			if err != nil {
				j.StateCtx.Logger().Warnf("could not build run status for job %d: %v. Run report will not execute", j.ID, err)
				continue
			}
			success, data, err := bundle.Reporter.RunReport(j.StateCtx, bundle.Parameters, runStatus, ev)
			if err != nil {
				j.StateCtx.Logger().Warnf("Run reporter failed while calculating run results, proceeding anyway: %v", err)
			} else {
				if success {
					j.StateCtx.Logger().Debugf("Run #%d of job %d considered successful according to %s", run+1, j.ID, bundle.Reporter.Name())
				} else {
					j.StateCtx.Logger().Errorf("Run #%d of job %d considered failed according to %s", run+1, j.ID, bundle.Reporter.Name())
				}
			}

			// TODO run report must be sent to the storage layer as soon as it's
			//      ready, not at the end of the job. This requires a change in
			//      how we store and expose reports, because this will require
			//      one DB entry per run report rather than one for all of them.
			r := job.Report{Success: success, Data: data, ReporterName: bundle.Reporter.Name(), ReportTime: time.Now()}
			runReports = append(runReports, &r)

		}
		allRunReports = append(allRunReports, runReports)

		if j.IsCancelled() {
			j.StateCtx.Logger().Debugf("Cancellation requested, skipping run #%d", run+1)
			break
		}
		// don't sleep on the last run
		if j.Runs == 0 || (j.Runs > 1 && run < j.Runs-1) {
			j.StateCtx.Logger().Infof("Sleeping %s before the next run...", j.RunInterval)
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

	for _, bundle := range j.FinalReporterBundles {
		// Build a RunStatus object for each run that we executed. We need to check if we interrupted
		// execution early and we did not perform all runs
		runStatuses, err := jr.BuildRunStatuses(j)
		if err != nil {
			j.StateCtx.Logger().Warnf("could not calculate run statuses: %v. Run report will not execute", err)
			continue
		}

		success, data, err := bundle.Reporter.FinalReport(j.StateCtx, bundle.Parameters, runStatuses, ev)
		if err != nil {
			j.StateCtx.Logger().Warnf("Final reporter failed while calculating test results, proceeding anyway: %v", err)
		} else {
			if success {
				j.StateCtx.Logger().Debugf("Job %d (%d runs out of %d desired) considered successful", j.ID, run, j.Runs)
			} else {
				j.StateCtx.Logger().Errorf("Job %d (%d runs out of %d desired) considered failed", j.ID, run, j.Runs)
			}
		}
		r := job.Report{Success: success, ReporterName: bundle.Reporter.Name(), ReportTime: time.Now(), Data: data}
		allFinalReports = append(allFinalReports, &r)
	}

	return allRunReports, allFinalReports, nil
}

// emitAcquiredTargets emits test events to keep track of Target acquisition
func (jr *JobRunner) emitAcquiredTargets(ctx xcontext.Context, emitter testevent.Emitter, targets []*target.Target) error {
	// The events hold a serialization of the Target in the payload
	for _, t := range targets {
		data := testevent.Data{EventName: target.EventTargetAcquired, Target: t}
		if err := emitter.Emit(ctx, data); err != nil {
			ctx.Logger().Warnf("could not emit event %s: %v", target.EventTargetAcquired, err)
			return err
		}
	}
	return nil
}

// GetCurrentRun returns the run which is currently being executed
func (jr *JobRunner) GetCurrentRun(ctx xcontext.Context, jobID types.JobID) (types.RunID, error) {

	var runID types.RunID

	runEvents, err := jr.frameworkEventManager.Fetch(ctx,
		frameworkevent.QueryJobID(jobID),
		frameworkevent.QueryEventName(EventRunStarted),
	)
	if err != nil {
		return runID, fmt.Errorf("could not fetch last run id for job %d: %v", jobID, err)
	}

	lastEvent := runEvents[len(runEvents)-1]
	payload := RunStartedPayload{}
	if err := json.Unmarshal([]byte(*lastEvent.Payload), &payload); err != nil {
		return runID, fmt.Errorf("could not fetch last run id for job %d: %v", jobID, err)
	}
	return payload.RunID, nil

}

func (jr *JobRunner) emitEvent(ctx xcontext.Context, jobID types.JobID, eventName event.Name, payload interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		ctx.Logger().Warnf("could not encode payload for event %s: %v", eventName, err)
		return err
	}

	rawPayload := json.RawMessage(payloadJSON)
	ev := frameworkevent.Event{JobID: jobID, EventName: eventName, Payload: &rawPayload, EmitTime: time.Now()}
	if err := jr.frameworkEventManager.Emit(ctx, ev); err != nil {
		ctx.Logger().Warnf("could not emit event %s: %v", eventName, err)
		return err
	}
	return nil
}

// NewJobRunner returns a new JobRunner, which holds an empty registry of jobs
func NewJobRunner() *JobRunner {
	jr := JobRunner{}
	jr.targetMap = make(map[types.JobID][]*target.Target)
	jr.targetLock = &sync.RWMutex{}
	jr.frameworkEventManager = storage.NewFrameworkEventEmitterFetcher()
	jr.testEvManager = storage.NewTestEventFetcher()
	return &jr
}
