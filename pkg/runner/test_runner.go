// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

// TestRunner is the interface test runner implements
type TestRunner interface {
	// ErrPaused will be returned when runner was able to pause successfully.
	// When this error is returned from Run(), it is accompanied by valid serialized state
	// that can be passed later to Run() to continue from where we left off.
	Run(ctx statectx.Context, test *test.Test, targets []*target.Target, jobID types.JobID, runID types.RunID, resumeState []byte) ([]byte, error)
}

// testRunner is the state associated with a test run.
// Here's how a test run works:
//  * Each target gets a targetState and a "target runner" - a goroutine that takes that particular
//    target through each step of the pipeline in sequence. It injects the target, waits for the result,
//    then mves on to the next step.
//  * Each step of the pipeline gets a stepState and:
//    - A "step runner" - a goroutine that is responsible for running the step's Run() method
//    - A "step reader" - a goroutine that processes results and sends them on to target runners that await them.
//  * After starting all of the above, the main goroutine goes into "monitor" mode
//    that checks on the pipeline's progress and is responsible for closing step input channels
//    when all the targets have been injected.
//  * Monitor loop finishes when all the targets have been injected into the last step
//    or if a step has encountered and error.
//  * We then wait for all the step runners and readers to shut down.
//  * Once all the activity has died down, resulting state is examined and an error is returned, if any.
type testRunner struct {
	stepInjectTimeout time.Duration // Time to wait for steps to accept each target
	shutdownTimeout   time.Duration // Time to wait for steps runners to finish a the end of the run

	steps     []*stepState            // The pipeline, in order of execution
	targets   map[string]*targetState // Target state lookup map
	targetsWg sync.WaitGroup          // Tracks all the target runners
	log       *logrus.Entry           // Logger

	// One mutex to rule them all, used to serialize access to all the state above.
	// Could probably be split into several if necessary.
	mu   sync.Mutex
	cond *sync.Cond // Used to notify the monitor about changes
}

// stepState contains state associated with one state of the pipeline:
type stepState struct {
	stepIndex int                 // Index of this step in the pipeline.
	sb        test.TestStepBundle // The test bundle.

	// Channels used to communicate with the plugin.
	inCh  chan *target.Target
	outCh chan *target.Target
	errCh chan cerrors.TargetError
	ev    testevent.Emitter

	numInjected int                     // Number of targets injected.
	tgtDone     map[*target.Target]bool // Targets for which results have been received.

	stepRunning   bool  // testStep.Run() is currently running.
	readerRunning bool  // Result reader is running.
	runErr        error // Runner error, returned from Run() or an error condition detected by the reader.

	log *logrus.Entry // Logger
}

// targetStepPhase denotes progression of a target through a step
type targetStepPhase int

const (
	targetStepPhaseInit  targetStepPhase = 0
	targetStepPhaseBegin targetStepPhase = 1 // Picked up for execution.
	targetStepPhaseRun   targetStepPhase = 2 // Injected into step.
	targetStepPhaseEnd   targetStepPhase = 3 // Finished running a step.
)

// targetState contains state associated with one target progressing through the pipeline.
type targetState struct {
	tgt *target.Target

	// This part of state gets serialized into JSON for resumption.
	CurStep  int             `json:"cur_step"`  // Current step number.
	CurPhase targetStepPhase `json:"cur_phase"` // Current phase of step execution.

	res   error      // Final result, if reached the end state.
	resCh chan error // Channel used to communicate result by the step runner.
}

// resumeStateStruct is used to serialize runner state to be resumed in the future.
type resumeStateStruct struct {
	Version int                     `json:"version"`
	JobID   types.JobID             `json:"job_id"`
	RunID   types.RunID             `json:"run_id"`
	Targets map[string]*targetState `json:"targets"`
}

// Resume state version we are compatible with.
// When imcompatible changes are made to the state format, bump this.
// Restoring incompatible state will abort the job.
const resumeStateStructVersion = 1

// Run is the main enty point of the runner.
func (tr *testRunner) Run(
	ctx statectx.Context,
	t *test.Test, targets []*target.Target,
	jobID types.JobID, runID types.RunID,
	resumeState []byte) ([]byte, error) {

	// Set up logger
	rootLog := logging.GetLogger("pkg/runner")
	fields := make(map[string]interface{})
	fields["jobid"] = jobID
	fields["runid"] = runID
	rootLog = logging.AddFields(rootLog, fields)
	tr.log = logging.AddField(rootLog, "phase", "run")

	tr.log.Debugf("== test runner starting job %d, run %d", jobID, runID)
	resumeState, err := tr.run(ctx, t, targets, jobID, runID, resumeState)
	tr.log.Debugf("== test runner finished job %d, run %d, err: %v", jobID, runID, err)
	return resumeState, err
}

func (tr *testRunner) run(
	ctx statectx.Context,
	t *test.Test, targets []*target.Target,
	jobID types.JobID, runID types.RunID,
	resumeState []byte) ([]byte, error) {

	// Peel off contexts used for steps and target handlers.
	stepCtx, _, stepCancel := statectx.WithParent(ctx)
	defer stepCancel()
	targetCtx, _, targetCancel := statectx.WithParent(ctx)
	defer targetCancel()

	// Set up the pipeline
	for i, sb := range t.TestStepsBundles {
		tr.steps = append(tr.steps, &stepState{
			stepIndex: i,
			sb:        sb,
			inCh:      make(chan *target.Target),
			outCh:     make(chan *target.Target),
			errCh:     make(chan cerrors.TargetError),
			ev: storage.NewTestEventEmitter(testevent.Header{
				JobID:         jobID,
				RunID:         runID,
				TestName:      t.Name,
				TestStepLabel: sb.TestStepLabel,
			}),
			tgtDone: make(map[*target.Target]bool),
			log:     logging.AddField(tr.log, "step", sb.TestStepLabel),
		})
	}

	// Set up the targets
	tr.targets = make(map[string]*targetState)
	// If we have target state to resume, do it now.
	if len(resumeState) > 0 {
		tr.log.Debugf("Attempting to resume from state: %s", string(resumeState))
		var rs resumeStateStruct
		if err := json.Unmarshal(resumeState, &rs); err != nil {
			return nil, fmt.Errorf("invalid resume state: %w", err)
		}
		if rs.Version != resumeStateStructVersion {
			return nil, fmt.Errorf("incompatible resume state version %d (want %d)",
				rs.Version, resumeStateStructVersion)
		}
		if rs.JobID != jobID {
			return nil, fmt.Errorf("wrong resume state, job id %d (want %d)", rs.JobID, jobID)
		}
		tr.targets = rs.Targets
	}
	// Initialize remaining fields of the target structures,
	// build the map and kick off target processing.
	for _, tgt := range targets {
		ts := tr.targets[tgt.ID]
		if ts == nil {
			ts = &targetState{}
		}
		ts.tgt = tgt
		ts.resCh = make(chan error)
		tr.mu.Lock()
		tr.targets[tgt.ID] = ts
		tr.mu.Unlock()
		tr.targetsWg.Add(1)
		go tr.targetRunner(targetCtx, stepCtx, ts)
	}

	// Run until no more progress can be made.
	runErr := tr.runMonitor()
	if runErr != nil {
		tr.log.Errorf("monitor returned error: %q, canceling", runErr)
		stepCancel()
	}

	// Wait for step runners and readers to exit.
	if err := tr.waitStepRunners(ctx); err != nil {
		tr.log.Errorf("step runner error: %q, canceling", err)
		stepCancel()
		if runErr == nil {
			runErr = err
		}
	}
	// There will be no more results, reel in all the target runners (if any).
	tr.log.Debugf("waiting for target runners to finish")
	targetCancel()
	tr.targetsWg.Wait()

	// Examine the resulting state.
	tr.log.Debugf("leaving, err %v, target states:", runErr)
	tr.mu.Lock()
	defer tr.mu.Unlock()
	resumeOk := (runErr == nil)
	var inFlightTargets []*targetState
	for i, tgt := range targets {
		ts := tr.targets[tgt.ID]
		stepErr := tr.steps[ts.CurStep].runErr
		tr.log.Debugf("  %d %s %v", i, ts, stepErr)
		if ts.CurPhase == targetStepPhaseRun {
			inFlightTargets = append(inFlightTargets, ts)
			if stepErr != statectx.ErrPaused {
				resumeOk = false
			}
		}
	}
	tr.log.Debugf("- %d in flight, ok to resume? %t", len(inFlightTargets), resumeOk)
	tr.log.Debugf("step states:")
	for i, ss := range tr.steps {
		tr.log.Debugf("  %d %s %t %t %v", i, ss, ss.stepRunning, ss.readerRunning, ss.runErr)
	}

	// Is there a useful error to report?
	if runErr != nil {
		return nil, runErr
	}

	// Has the run been canceled?
	select {
	case <-ctx.Done():
		return nil, statectx.ErrCanceled
	default:
	}

	// Have we been asked to pause? If yes, is it safe to do so?
	select {
	case <-ctx.Paused():
		if !resumeOk {
			tr.log.Warningf("paused but not ok to resume")
			break
		}
		rs := &resumeStateStruct{
			Version: resumeStateStructVersion,
			JobID:   jobID, RunID: runID,
			Targets: tr.targets,
		}
		resumeState, runErr = json.Marshal(rs)
		if runErr != nil {
			tr.log.Errorf("unable to serialize the state: %s", runErr)
		} else {
			runErr = statectx.ErrPaused
		}
	default:
		// We are not pausing and yet some targets were left in flight.
		if len(inFlightTargets) > 0 {
			ts := inFlightTargets[0]
			runErr = &cerrors.ErrTestStepLostTargets{
				StepName: tr.steps[ts.CurStep].sb.TestStepLabel,
				Target:   ts.tgt.ID,
			}
		}
	}

	return resumeState, runErr
}

func (tr *testRunner) waitStepRunners(ctx statectx.Context) error {
	tr.log.Debugf("waiting for step runners to finish")
	swch := make(chan struct{})
	go func() {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		for {
			ok := true
			for _, ss := range tr.steps {
				// numRunning == 1 is also acceptable: we allow the Run() goroutine
				// to continue in case of error, if the result processor decided
				// to abandon its runner, there's nothing we can do.
				switch {
				case !ss.stepRunning && !ss.readerRunning:
				// Done
				case ss.stepRunning && ss.readerRunning:
					// Still active
					ok = false
				case !ss.stepRunning && ss.readerRunning:
					// Transient state, let it finish
					ok = false
				case ss.stepRunning && !ss.readerRunning:
					// This is possible if plugin got stuck and result processor gave up on it.
					// If so, it should have left an error.
					if ss.runErr == nil {
						tr.log.Errorf("%s: result processor left runner with no error", ss)
						// There's nothing we can do at this point, fall through.
					}
				}
			}
			if ok {
				close(swch)
				return
			}
			tr.cond.Wait()
		}
	}()
	var err error
	select {
	case <-swch:
		tr.log.Debugf("step runners finished")
		tr.mu.Lock()
		defer tr.mu.Unlock()
		err = tr.checkStepRunners()
	case <-time.After(tr.shutdownTimeout):
		tr.log.Errorf("step runners failed to shut down correctly")
		tr.mu.Lock()
		defer tr.mu.Unlock()
		// If there is a step with an error set, use that.
		err = tr.checkStepRunners()
		// If there isn't, enumerate ones that were still running at the time.
		nrerr := &cerrors.ErrTestStepsNeverReturned{}
		if err == nil {
			err = nrerr
		}
		for _, ss := range tr.steps {
			if ss.stepRunning {
				nrerr.StepNames = append(nrerr.StepNames, ss.sb.TestStepLabel)
				// We cannot make the step itself return but we can at least release the reader.
				tr.safeCloseOutCh(ss)
			}
		}
	}
	// Emit step error events.
	for _, ss := range tr.steps {
		if ss.runErr != nil && ss.runErr != statectx.ErrPaused && ss.runErr != statectx.ErrCanceled {
			if err := ss.emitEvent(EventTestError, nil, ss.runErr.Error()); err != nil {
				tr.log.Errorf("failed to emit event: %s", err)
			}
		}
	}
	return err
}

// targetRunner runs one target through all the steps of the pipeline.
func (tr *testRunner) targetRunner(ctx, stepCtx statectx.Context, ts *targetState) {
	log := logging.AddField(tr.log, "target", ts.tgt.ID)
	log.Debugf("%s: target runner active", ts)
	// NB: CurStep may be non-zero on entry if resumed
loop:
	for i := ts.CurStep; i < len(tr.steps); {
		// Early check for pause of cancelation.
		select {
		case <-ctx.Paused():
			log.Debugf("%s: paused 0", ts)
			break loop
		case <-ctx.Done():
			log.Debugf("%s: canceled 0", ts)
			break loop
		default:
		}
		tr.mu.Lock()
		ss := tr.steps[i]
		if ts.CurPhase == targetStepPhaseEnd {
			// This target already terminated.
			// Can happen if resumed from terminal state.
			tr.mu.Unlock()
			break loop
		}
		ts.CurStep = i
		ts.CurPhase = targetStepPhaseBegin
		tr.mu.Unlock()
		// Make sure we have a step runner active.
		// These are started on-demand.
		tr.runStepIfNeeded(stepCtx, ss)
		// Inject the target.
		log.Debugf("%s: injecting into %s", ts, ss)
		select {
		case ss.inCh <- ts.tgt:
			// Injected successfully.
			err := ss.ev.Emit(testevent.Data{EventName: target.EventTargetIn, Target: ts.tgt})
			tr.mu.Lock()
			ts.CurPhase = targetStepPhaseRun
			ss.numInjected++
			if err != nil {
				ss.runErr = fmt.Errorf("failed to report target injection: %w", err)
				ss.log.Errorf("%s", ss.runErr)
			}
			tr.mu.Unlock()
			tr.cond.Signal()
			if err != nil {
				break loop
			}
		case <-time.After(tr.stepInjectTimeout):
			tr.mu.Lock()
			ss.log.Errorf("timed out while injecting a target")
			ss.runErr = &cerrors.ErrTestTargetInjectionTimedOut{StepName: ss.sb.TestStepLabel}
			tr.mu.Unlock()
			err := ss.ev.Emit(testevent.Data{EventName: target.EventTargetInErr, Target: ts.tgt})
			if err != nil {
				ss.log.Errorf("failed to emit event: %s", err)
			}
			break loop
		case <-ctx.Done():
			log.Debugf("%s: canceled 1", ts)
			break loop
		}
		// Await result. It will be communicated to us by the step runner.
		select {
		case res, ok := <-ts.resCh:
			if !ok {
				log.Debugf("%s: result channel closed", ts)
				break loop
			}
			log.Debugf("%s: result for %s recd", ts, ss)
			var err error
			if res == nil {
				err = ss.emitEvent(target.EventTargetOut, ts.tgt, nil)
			} else {
				err = ss.emitEvent(target.EventTargetErr, ts.tgt, target.ErrPayload{Error: res.Error()})
			}
			if err != nil {
				ss.log.Errorf("failed to emit event: %s", err)
			}
			tr.mu.Lock()
			ts.CurPhase = targetStepPhaseEnd
			ts.res = res
			tr.cond.Signal()
			if res != nil {
				tr.mu.Unlock()
				break loop
			}
			i++
			if i < len(tr.steps) {
				ts.CurStep = i
				ts.CurPhase = targetStepPhaseInit
			}
			tr.mu.Unlock()
			// Check for cancellation.
			// Notably we are not checking for the pause condition here:
			// when paused, we want to let all the injected targets to finish
			// and collect all the results they produce. If that doesn't happen,
			// step runner will close resCh on its way out and unlock us.
		case <-ctx.Done():
			log.Debugf("%s: canceled 2", ts)
			break loop
		}
	}
	log.Debugf("%s: target runner finished", ts)
	tr.mu.Lock()
	ts.resCh = nil
	tr.cond.Signal()
	tr.mu.Unlock()
	tr.targetsWg.Done()
}

// runStepIfNeeded starts the step runner goroutine if not already running.
func (tr *testRunner) runStepIfNeeded(ctx statectx.Context, ss *stepState) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if ss.stepRunning {
		return
	}
	if ss.runErr != nil {
		return
	}
	ss.stepRunning = true
	ss.readerRunning = true
	go tr.stepRunner(ctx, ss)
	go tr.stepReader(ctx, ss)
}

// emitEvent emits the specified event with the specified JSON payload (if any).
func (ss *stepState) emitEvent(name event.Name, tgt *target.Target, payload interface{}) error {
	var payloadJSON *json.RawMessage
	if payload != nil {
		payloadBytes, jmErr := json.Marshal(payload)
		if jmErr != nil {
			return fmt.Errorf("failed to marshal event: %w", jmErr)
		}
		pj := json.RawMessage(payloadBytes)
		payloadJSON = &pj
	}
	errEv := testevent.Data{
		EventName: name,
		Target:    tgt,
		Payload:   payloadJSON,
	}
	return ss.ev.Emit(errEv)
}

// stepRunner runs a test pipeline's step (the Run() method).
func (tr *testRunner) stepRunner(ctx statectx.Context, ss *stepState) {
	ss.log.Debugf("%s: step runner active", ss)
	defer func() {
		if r := recover(); r != nil {
			tr.mu.Lock()
			ss.stepRunning = false
			ss.runErr = &cerrors.ErrTestStepPaniced{
				StepName:   ss.sb.TestStepLabel,
				StackTrace: fmt.Sprintf("%s / %s", r, debug.Stack()),
			}
			tr.mu.Unlock()
			tr.safeCloseOutCh(ss)
		}
	}()
	chans := test.TestStepChannels{In: ss.inCh, Out: ss.outCh, Err: ss.errCh}
	runErr := ss.sb.TestStep.Run(ctx, chans, ss.sb.Parameters, ss.ev)
	tr.mu.Lock()
	ss.stepRunning = false
	if runErr != nil {
		ss.runErr = runErr
	}
	tr.mu.Unlock()
	// Signal to the result processor that no more will be coming.
	tr.safeCloseOutCh(ss)
	ss.log.Debugf("%s: step runner finished", ss)
}

// reportTargetResult reports result of executing a step to the appropriate target runner.
func (tr *testRunner) reportTargetResult(ctx statectx.Context, ss *stepState, tgt *target.Target, res error) error {
	resCh, err := func() (chan error, error) {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		ts := tr.targets[tgt.ID]
		if ts == nil {
			return nil, fmt.Errorf("%s: result for nonexistent target %s %v", ss, tgt, res)
		}
		if ss.tgtDone[tgt] {
			return nil, &cerrors.ErrTestStepReturnedDuplicateResult{
				StepName: ss.sb.TestStepLabel,
				Target:   tgt.ID,
			}
		}
		ss.tgtDone[tgt] = true
		// Begin is also allowed here because it may happen that we get a result before target runner updates phase.
		if ts.CurStep != ss.stepIndex || (ts.CurPhase != targetStepPhaseBegin && ts.CurPhase != targetStepPhaseRun) {
			return nil, &cerrors.ErrTestStepReturnedUnexpectedResult{
				StepName: ss.sb.TestStepLabel,
				Target:   tgt.ID,
			}
		}
		if ts.resCh == nil {
			// This should not happen, must be an internal error.
			return nil, fmt.Errorf("%s: target runner %s is not there, dropping result on the floor", ss, ts)
		}
		ss.log.Debugf("%s: result for %s: %v", ss, ts, res)
		return ts.resCh, nil
	}()
	if err != nil {
		return err
	}
	select {
	case resCh <- res:
		break
	case <-ctx.Done():
		break
	}
	return nil
}

func (tr *testRunner) safeCloseOutCh(ss *stepState) {
	defer func() {
		if r := recover(); r != nil {
			tr.mu.Lock()
			ss.runErr = &cerrors.ErrTestStepClosedChannels{StepName: ss.sb.TestStepLabel}
			tr.mu.Unlock()
		}
	}()
	close(ss.outCh)
}

// safeCloseErrCh closes error channel safely, even if it has already been closed.
func (tr *testRunner) safeCloseErrCh(ss *stepState) {
	defer func() {
		if r := recover(); r != nil {
			tr.mu.Lock()
			ss.runErr = &cerrors.ErrTestStepClosedChannels{StepName: ss.sb.TestStepLabel}
			tr.mu.Unlock()
		}
	}()
	close(ss.errCh)
}

// stepReader receives results from the step's output channel and forwards them to the appropriate target runners.
func (tr *testRunner) stepReader(ctx statectx.Context, ss *stepState) {
	ss.log.Debugf("%s: step reader active", ss)
	var err error
	outCh := ss.outCh
loop:
	for {
		select {
		case tgt, ok := <-outCh:
			if !ok {
				ss.log.Debugf("%s: out chan closed", ss)
				// At this point we may still have an error to report,
				// wait until error channel is emptied too.
				outCh = make(chan *target.Target)
				tr.safeCloseErrCh(ss)
				break
			}
			if err = tr.reportTargetResult(ctx, ss, tgt, nil); err != nil {
				break loop
			}
		case res, ok := <-ss.errCh:
			if !ok {
				ss.log.Debugf("%s: err chan closed", ss)
				break loop
			}
			if err = tr.reportTargetResult(ctx, ss, res.Target, res.Err); err != nil {
				break loop
			}
		case <-ctx.Done():
			ss.log.Debugf("%s: canceled 3, draining", ss)
			for {
				select {
				case <-outCh:
					break loop
				case <-ss.errCh:
					break loop
				case <-time.After(tr.shutdownTimeout):
					tr.mu.Lock()
					if ss.runErr == nil {
						ss.runErr = &cerrors.ErrTestStepsNeverReturned{}
					}
					tr.mu.Unlock()
				}
			}
		}
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if ss.runErr == nil && err != nil {
		ss.runErr = err
	}
	if ss.stepRunning && ss.runErr == nil {
		// This means that plugin closed its channels before leaving.
		ss.runErr = &cerrors.ErrTestStepClosedChannels{StepName: ss.sb.TestStepLabel}
	}
	ss.readerRunning = false
	ss.log.Debugf("%s: step reader finished, %t %t %v", ss, ss.stepRunning, ss.readerRunning, ss.runErr)
	tr.cond.Signal()
}

// checkStepRunnersLocked checks if any step runner has encountered an error.
func (tr *testRunner) checkStepRunners() error {
	for _, ss := range tr.steps {
		if ss.runErr != nil {
			return ss.runErr
		}
	}
	return nil
}

// runMonitor monitors progress of targets through the pipeline
// and closes input channels of the steps to indicate that no more are expected.
// It also monitors steps for critical errors and cancels the whole run.
// Note: input channels remain open when cancellation is requested,
// plugins are expected to handle it explicitly.
func (tr *testRunner) runMonitor() error {
	tr.log.Debugf("monitor: active")
	tr.mu.Lock()
	defer tr.mu.Unlock()
	// First, compute the starting step of the pipeline (it may be non-zero
	// if the pipleine was resumed).
	minStep := len(tr.steps)
	for _, ts := range tr.targets {
		if ts.CurStep < minStep {
			minStep = ts.CurStep
		}
	}
	if minStep < len(tr.steps) {
		tr.log.Debugf("monitor: starting at step %s", tr.steps[minStep])
	}

	// Run the main loop.
	pass := 1
	var runErr error
loop:
	for step := minStep; step < len(tr.steps); pass++ {
		ss := tr.steps[step]
		tr.log.Debugf("monitor pass %d: current step %s", pass, ss)
		// Check if all the targets have either made it past the injection phase or terminated.
		ok := true
		for _, ts := range tr.targets {
			tr.log.Debugf("monitor pass %d: %s: %s", pass, ss, ts)
			if ts.resCh == nil { // Not running anymore
				continue
			}
			if ok && (ts.CurStep < step || ts.CurPhase < targetStepPhaseRun) {
				tr.log.Debugf("monitor pass %d: %s: not all targets injected yet (%s)", pass, ss, ts)
				ok = false
				break
			}
		}
		if runErr = tr.checkStepRunners(); runErr != nil {
			break loop
		}
		if !ok {
			// Wait for notification: as progress is being made, we get notified.
			tr.cond.Wait()
			continue
		}
		// All targets ok, close the step's input channel.
		tr.log.Debugf("monitor pass %d: %s: no more targets, closing input channel", pass, ss)
		close(ss.inCh)
		step++
	}
	tr.log.Debugf("monitor: finished, %v", runErr)
	return runErr
}

func NewTestRunnerWithTimeouts(stepInjectTimeout, shutdownTimeout time.Duration) TestRunner {
	tr := &testRunner{
		stepInjectTimeout: stepInjectTimeout,
		shutdownTimeout:   shutdownTimeout,
	}
	tr.cond = sync.NewCond(&tr.mu)
	return tr
}

func NewTestRunner() TestRunner {
	return NewTestRunnerWithTimeouts(config.StepInjectTimeout, config.TestRunnerShutdownTimeout)
}

func (tph targetStepPhase) String() string {
	switch tph {
	case targetStepPhaseInit:
		return "init"
	case targetStepPhaseBegin:
		return "begin"
	case targetStepPhaseRun:
		return "run"
	case targetStepPhaseEnd:
		return "end"
	}
	return fmt.Sprintf("???(%d)", tph)
}

func (ss *stepState) String() string {
	return fmt.Sprintf("[#%d %s]", ss.stepIndex, ss.sb.TestStepLabel)
}

func (ts *targetState) String() string {
	var resText string
	if ts.res != nil {
		resStr := fmt.Sprintf("%s", ts.res)
		if len(resStr) > 20 {
			resStr = resStr[:20] + "..."
		}
		resText = fmt.Sprintf("%q", resStr)
	} else {
		resText = "<nil>"
	}
	finished := ts.resCh == nil
	return fmt.Sprintf("[%s %d %s %t %s]",
		ts.tgt, ts.CurStep, ts.CurPhase, finished, resText)
}
