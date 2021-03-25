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
func (jr *JobRunner) Run(ctx xcontext.Context, j *job.Job, resumeState *job.PauseEventPayload) ([][]*job.Report, []*job.Report, *job.PauseEventPayload, error) {
	runID := types.RunID(1)
	var delay time.Duration

	jobCtx := ctx.WithFields(xcontext.Fields{"jobid": j.ID})

	if resumeState != nil {
		runID = resumeState.RunID
		if resumeState.StartAt != nil {
			// This may get negative. It's fine.
			delay = time.Until(*resumeState.StartAt)
		}
	}

	if j.Runs == 0 {
		ctx.Infof("Running job '%s' (id %v) indefinitely, current run #%d", j.Name, j.ID, runID)
	} else {
		ctx.Infof("Running job '%s' %d times, starting at #%d", j.Name, j.Runs, runID)
	}
	tl := target.GetLocker()
	ev := storage.NewTestEventFetcher()

	var (
		runReports      []*job.Report
		allRunReports   [][]*job.Report
		allFinalReports []*job.Report
		runErr          error
	)

	for ; runID <= types.RunID(j.Runs) || j.Runs == 0; runID++ {
		runCtx := ctx.WithFields(xcontext.Fields{"runid": runID})
		if delay > 0 {
			nextRun := time.Now().Add(delay)
			runCtx.Infof("Sleeping %s before the next run...", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Until(xcontext.ErrPaused):
				resumeState = &job.PauseEventPayload{
					Version: job.CurrentPauseEventPayloadVersion,
					JobID:   j.ID,
					RunID:   runID,
					StartAt: &nextRun,
				}
				return nil, nil, resumeState, xcontext.ErrPaused
			case <-ctx.Done():
				return nil, nil, nil, xcontext.ErrCanceled
			}
		}

		// If we can't emit the run start event, we ignore the error. The framework will
		// try to rebuild the status if it detects that an event might have gone missing
		payload := RunStartedPayload{RunID: runID}
		err := jr.emitEvent(runCtx, j.ID, EventRunStarted, payload)
		if err != nil {
			runCtx.Warnf("Could not emit event run (run %d) start for job %d: %v", runID, j.ID, err)
		}

		for idx, t := range j.Tests {
			runCtx.Infof("Run #%d: fetching targets for test '%s'", runID, t.Name)
			bundle := t.TargetManagerBundle
			var (
				targets  []*target.Target
				errCh    = make(chan error, 1)
				acquired = false
			)
			// the Acquire semantic is synchronous, so that the implementation
			// is simpler on the user's side. We run it in a goroutine in
			// order to use a timeout for target acquisition.
			go func() {
				// Skip target acquisition if resuming existing run.
				if resumeState != nil {
					targets = resumeState.Targets
				}
				if len(targets) == 0 {
					targets, err = bundle.TargetManager.Acquire(ctx, j.ID, j.TargetManagerAcquireTimeout+config.LockRefreshTimeout, bundle.AcquireParameters, tl)
					if err != nil {
						errCh <- err
						return
					}
					acquired = true
				}
				// Lock all the targets returned by Acquire.
				// Targets can also be locked in the `Acquire` method, for
				// example to allow dynamic acquisition.
				// We lock them again to ensure that all the acquired
				// targets are locked before running the job.
				// Locking an already-locked target (by the same owner)
				// extends the locking deadline.
				if err := tl.Lock(runCtx, j.ID, j.TargetManagerAcquireTimeout, targets); err != nil {
					errCh <- fmt.Errorf("Target locking failed: %w", err)
				}
				errCh <- nil
			}()
			// wait for targets up to a certain amount of time
			select {
			case err := <-errCh:
				if err != nil {
					err = fmt.Errorf("run #%d: cannot fetch targets for test '%s': %v", runID, t.Name, err)
					runCtx.Errorf(err.Error())
					return nil, nil, nil, err
				}
				// Associate the targets with the job for later retrievel
				jr.targetLock.Lock()
				jr.targetMap[j.ID] = targets
				jr.targetLock.Unlock()

			case <-time.After(j.TargetManagerAcquireTimeout):
				return nil, nil, nil, fmt.Errorf("target manager acquire timed out after %s", j.TargetManagerAcquireTimeout)
			case <-ctx.Until(xcontext.ErrPaused):
				runCtx.Infof("pause requested for job ID %v", j.ID)
				resumeState = &job.PauseEventPayload{
					Version: job.CurrentPauseEventPayloadVersion,
					JobID:   j.ID,
					RunID:   runID,
				}
				return nil, nil, resumeState, xcontext.ErrPaused
			case <-ctx.Done():
				runCtx.Infof("cancellation requested for job ID %v", j.ID)
				return nil, nil, nil, xcontext.ErrCanceled
			}

			// refresh the target locks periodically, by extending their
			// expiration time. If the job is cancelled, the locks are released.
			// If the job is paused (e.g. because we are migrating the ConTest
			// instance or upgrading it), the locks are not released, because we
			// may want to resume once the new ConTest instance starts.
			stopRefresh := make(chan struct{})
			stoppedRefresh := make(chan struct{})
			go func(j *job.Job, tl target.Locker, targets []*target.Target, refreshInterval time.Duration) {
				for {
					select {
					case <-time.After(refreshInterval):
						// refresh the locks before the timeout expires
						if err := tl.RefreshLocks(runCtx, j.ID, targets); err != nil {
							runCtx.Warnf("Failed to refresh %d locks for job ID %d: %v", len(targets), j.ID, err)
						}
					// Stop refreshing when told.
					case <-stopRefresh:
						close(stoppedRefresh)
						return
					}
				}
				// refresh locks a bit faster than locking timeout to avoid races
			}(j, tl, targets, config.LockRefreshTimeout/10*9)

			// Emit events tracking targets acquisition
			header := testevent.Header{JobID: j.ID, RunID: runID, TestName: t.Name}
			testEventEmitter := storage.NewTestEventEmitter(header)

			if acquired {
				runErr = jr.emitTargetEvents(runCtx, testEventEmitter, targets, target.EventTargetAcquired)
			}

			if runErr == nil {
				runCtx.Infof("Run #%d: running test #%d for job '%s' (job ID: %d) on %d targets",
					runID, idx, j.Name, j.ID, len(targets))
				testRunner := NewTestRunner()
				var testRunnerState json.RawMessage
				if resumeState != nil {
					testRunnerState = resumeState.TestRunnerState
				}
				testRunnerState, err := testRunner.Run(ctx, t, targets, j.ID, runID, testRunnerState)
				if err == xcontext.ErrPaused {
					resumeState := &job.PauseEventPayload{
						Version:         job.CurrentPauseEventPayloadVersion,
						JobID:           j.ID,
						RunID:           runID,
						Targets:         targets,
						TestRunnerState: testRunnerState,
					}
					// Stop refreshing the locks.
					close(stopRefresh)
					<-stoppedRefresh
					// Refresh one last time.
					if err2 := tl.RefreshLocks(runCtx, j.ID, targets); err2 != nil {
						runCtx.Errorf("Failed to refresh %d locks for job ID %d: %v", len(targets), j.ID, err2)
						resumeState = nil
						err = err2
					}
					// Return without releasing targets.
					return nil, nil, resumeState, err
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
				err := bundle.TargetManager.Release(ctx, j.ID, bundle.ReleaseParameters)
				// Announce that targets have been released
				_ = jr.emitTargetEvents(runCtx, testEventEmitter, targets, target.EventTargetReleased)
				// signal that we are done to the goroutine that refreshes the locks.
				close(stopRefresh)
				<-stoppedRefresh
				if err := tl.Unlock(runCtx, j.ID, targets); err == nil {
					runCtx.Infof("Unlocked %d target(s) for job ID %d", len(targets), j.ID)
				} else {
					runCtx.Warnf("Failed to unlock %d target(s) (%v): %v", len(targets), targets, err)
				}
				errCh <- err
			}()
			select {
			case err := <-errCh:
				if err != nil {
					errRelease := fmt.Sprintf("Failed to release targets: %v", err)
					runCtx.Errorf(errRelease)
					return nil, nil, nil, fmt.Errorf(errRelease)
				}
			case <-time.After(j.TargetManagerReleaseTimeout):
				return nil, nil, nil, fmt.Errorf("target manager release timed out after %s", j.TargetManagerReleaseTimeout)
				// Ignore cancellation here, we want release and unlock to happen in that case.
			}
			// return the Run error only after releasing the targets, and only
			// if we are not running indefinitely. An error returned by the TestRunner
			// is considered a fatal condition and will cause the termination of the
			// whole job.
			if runErr != nil {
				return nil, nil, nil, runErr
			}
		}

		delay = j.RunInterval
		resumeState = nil
	}

	// Prepare reports.

	for runID = 1; runID <= types.RunID(j.Runs); runID++ {
		// Calculate results for this run via the registered run reporters reporters
		runCoordinates := job.RunCoordinates{JobID: j.ID, RunID: runID}

		runReports = make([]*job.Report, 0, len(j.RunReporterBundles))
		for _, bundle := range j.RunReporterBundles {
			runStatus, err := jr.BuildRunStatus(jobCtx, runCoordinates, j)
			if err != nil {
				jobCtx.Warnf("could not build run status for job %d: %v. Run report will not execute", j.ID, err)
				continue
			}
			success, data, err := bundle.Reporter.RunReport(ctx, bundle.Parameters, runStatus, ev)
			if err != nil {
				jobCtx.Warnf("Run reporter failed while calculating run results, proceeding anyway: %v", err)
			} else {
				if success {
					jobCtx.Infof("Run #%d of job %d considered successful according to %s", runID, j.ID, bundle.Reporter.Name())
				} else {
					jobCtx.Errorf("Run #%d of job %d considered failed according to %s", runID, j.ID, bundle.Reporter.Name())
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
	}

	for _, bundle := range j.FinalReporterBundles {
		// Build a RunStatus object for each run that we executed. We need to check if we interrupted
		// execution early and we did not perform all runs
		runStatuses, err := jr.BuildRunStatuses(jobCtx, j)
		if err != nil {
			jobCtx.Warnf("could not calculate run statuses: %v. Run report will not execute", err)
			continue
		}

		success, data, err := bundle.Reporter.FinalReport(ctx, bundle.Parameters, runStatuses, ev)
		if err != nil {
			jobCtx.Warnf("Final reporter failed while calculating test results, proceeding anyway: %v", err)
		} else {
			if success {
				jobCtx.Infof("Job %d (%d runs out of %d desired) considered successful", j.ID, runID-1, j.Runs)
			} else {
				jobCtx.Errorf("Job %d (%d runs out of %d desired) considered failed", j.ID, runID-1, j.Runs)
			}
		}
		r := job.Report{Success: success, ReporterName: bundle.Reporter.Name(), ReportTime: time.Now(), Data: data}
		allFinalReports = append(allFinalReports, &r)
	}

	return allRunReports, allFinalReports, nil, nil
}

// emitTargetEvents emits test events to keep track of Target acquisition and release
func (jr *JobRunner) emitTargetEvents(ctx xcontext.Context, emitter testevent.Emitter, targets []*target.Target, eventName event.Name) error {
	// The events hold a serialization of the Target in the payload
	for _, t := range targets {
		data := testevent.Data{EventName: eventName, Target: t}
		if err := emitter.Emit(ctx, data); err != nil {
			ctx.Warnf("could not emit event %s: %v", eventName, err)
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
		ctx.Warnf("could not encode payload for event %s: %v", eventName, err)
		return err
	}

	rawPayload := json.RawMessage(payloadJSON)
	ev := frameworkevent.Event{JobID: jobID, EventName: eventName, Payload: &rawPayload, EmitTime: time.Now()}
	if err := jr.frameworkEventManager.Emit(ctx, ev); err != nil {
		ctx.Warnf("could not emit event %s: %v", eventName, err)
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
