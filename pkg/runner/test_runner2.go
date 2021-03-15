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

	"github.com/insomniacslk/xjson"
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

// TestRunner is the state associated with a test run.
// Here's how a test run works:
//  * Each target gets a targetState and a "target handler" - a goroutine that takes that particular
//    target through each step of the pipeline in sequence. It injects the target, waits for the result,
//    then moves on to the next step.
//  * Each step of the pipeline gets a stepState and:
//    - A "step runner" - a goroutine that is responsible for running the step's Run() method
//    - A "step reader" - a goroutine that processes results and sends them on to target handlers that await them.
//  * After starting all of the above, the main goroutine goes into "monitor" mode
//    that checks on the pipeline's progress and is responsible for closing step input channels
//    when all the targets have been injected.
//  * Monitor loop finishes when all the targets have been injected into the last step
//    or if a step has encountered an error.
//  * We then wait for all the step runners and readers to shut down.
//  * Once all the activity has died down, resulting state is examined and an error is returned, if any.
type TestRunner struct {
	shutdownTimeout time.Duration // Time to wait for steps runners to finish a the end of the run

	steps     []*stepState            // The pipeline, in order of execution
	targets   map[string]*targetState // Target state lookup map
	targetsWg sync.WaitGroup          // Tracks all the target handlers
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

	tgtDone map[*target.Target]bool // Targets for which results have been received.

	stepRunning   bool  // testStep.Run() is currently running.
	readerRunning bool  // Result reader is running.
	runErr        error // Runner error, returned from Run() or an error condition detected by the reader.

	log *logrus.Entry // Logger
}

// targetStepPhase denotes progression of a target through a step
type targetStepPhase int

const (
	targetStepPhaseInvalid       targetStepPhase = iota
	targetStepPhaseInit                          // (1) Created
	targetStepPhaseBegin                         // (2) Picked up for execution.
	targetStepPhaseRun                           // (3) Injected into step.
	targetStepPhaseResultPending                 // (4) Result posted to the handler.
	targetStepPhaseEnd                           // (5) Finished running a step.
)

// targetState contains state associated with one target progressing through the pipeline.
type targetState struct {
	tgt *target.Target

	// This part of state gets serialized into JSON for resumption.
	CurStep  int             `json:"cs"`            // Current step number.
	CurPhase targetStepPhase `json:"cp"`            // Current phase of step execution.
	Res      *xjson.Error    `json:"res,omitempty"` // Final result, if reached the end state.

	resCh chan error // Channel used to communicate result by the step runner.
}

// resumeStateStruct is used to serialize runner state to be resumed in the future.
type resumeStateStruct struct {
	Version int                     `json:"v"`
	JobID   types.JobID             `json:"job_id"`
	RunID   types.RunID             `json:"run_id"`
	Targets map[string]*targetState `json:"targets"`
}

// Resume state version we are compatible with.
// When imcompatible changes are made to the state format, bump this.
// Restoring incompatible state will abort the job.
const resumeStateStructVersion = 2

// Run is the main enty point of the runner.
func (tr *TestRunner) Run(
	ctx statectx.Context,
	t *test.Test, targets []*target.Target,
	jobID types.JobID, runID types.RunID,
	resumeState json.RawMessage) (json.RawMessage, error) {

	// Set up logger
	rootLog := logging.GetLogger("TestRunner")
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

func (tr *TestRunner) run(
	ctx statectx.Context,
	t *test.Test, targets []*target.Target,
	jobID types.JobID, runID types.RunID,
	resumeState json.RawMessage) (json.RawMessage, error) {

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
		// Step handlers will be started from target handlers as targets reach them.
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
		tr.mu.Lock()
		tgs := tr.targets[tgt.ID]
		if tgs == nil {
			tgs = &targetState{
				CurPhase: targetStepPhaseInit,
			}
		}
		tgs.tgt = tgt
		// Buffer of 1 is needed so that the step reader does not block when submitting result back
		// to the target handler. Target handler may not yet be ready to receive the result,
		// i.e. reporting TargetIn event which may involve network I/O.
		tgs.resCh = make(chan error, 1)
		tr.targets[tgt.ID] = tgs
		tr.mu.Unlock()
		tr.targetsWg.Add(1)
		go func() {
			tr.targetHandler(targetCtx, stepCtx, tgs)
			tr.targetsWg.Done()
		}()
	}

	// Run until no more progress can be made.
	runErr := tr.runMonitor()
	if runErr != nil {
		tr.log.Errorf("monitor returned error: %q, canceling", runErr)
		stepCancel()
	}

	// Wait for step runners and readers to exit.
	if err := tr.waitStepRunners(ctx); err != nil {
		if runErr == nil {
			runErr = err
		}
	}

	// There will be no more results, reel in all the target handlers (if any).
	tr.log.Debugf("waiting for target handlers to finish")
	targetCancel()
	tr.targetsWg.Wait()

	// Examine the resulting state.
	tr.log.Debugf("leaving, err %v, target states:", runErr)
	tr.mu.Lock()
	defer tr.mu.Unlock()
	resumeOk := (runErr == nil)
	numInFlightTargets := 0
	for i, tgt := range targets {
		tgs := tr.targets[tgt.ID]
		stepErr := tr.steps[tgs.CurStep].runErr
		tr.log.Debugf("  %d %s %v", i, tgs, stepErr)
		if tgs.CurPhase == targetStepPhaseRun {
			numInFlightTargets++
		}
		if stepErr != nil && stepErr != statectx.ErrPaused {
			resumeOk = false
		}
	}
	tr.log.Debugf("- %d in flight, ok to resume? %t", numInFlightTargets, resumeOk)
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
	}

	return resumeState, runErr
}

func (tr *TestRunner) waitStepRunners(ctx statectx.Context) error {
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
		tr.log.Debugf("%s %v", ss, ss.runErr)
		if ss.runErr != nil && ss.runErr != statectx.ErrPaused && ss.runErr != statectx.ErrCanceled {
			if err := ss.emitEvent(EventTestError, nil, ss.runErr.Error()); err != nil {
				tr.log.Errorf("failed to emit event: %s", err)
			}
		}
	}
	return err
}

func (tr *TestRunner) injectTarget(ctx statectx.Context, tgs *targetState, ss *stepState, log *logrus.Entry) error {
	log.Debugf("%s: injecting into %s", tgs, ss)
	select {
	case ss.inCh <- tgs.tgt:
		// Injected successfully.
		err := ss.ev.Emit(testevent.Data{EventName: target.EventTargetIn, Target: tgs.tgt})
		tr.mu.Lock()
		defer tr.mu.Unlock()
		// By the time we get here the target could have been processed and result posted already, hence the check.
		if tgs.CurPhase == targetStepPhaseBegin {
			tgs.CurPhase = targetStepPhaseRun
		}
		if err != nil {
			return fmt.Errorf("failed to report target injection: %w", err)
		}
		tr.cond.Signal()
	case <-ctx.Done():
		return statectx.ErrCanceled
	}
	return nil
}

func (tr *TestRunner) awaitTargetResult(ctx statectx.Context, tgs *targetState, ss *stepState, log *logrus.Entry) error {
	select {
	case res, ok := <-tgs.resCh:
		if !ok {
			log.Debugf("%s: result channel closed", tgs)
			return statectx.ErrCanceled
		}
		log.Debugf("%s: result recd for %s", tgs, ss)
		var err error
		if res == nil {
			err = ss.emitEvent(target.EventTargetOut, tgs.tgt, nil)
		} else {
			err = ss.emitEvent(target.EventTargetErr, tgs.tgt, target.ErrPayload{Error: res.Error()})
		}
		if err != nil {
			ss.log.Errorf("failed to emit event: %s", err)
		}
		tr.mu.Lock()
		if res != nil {
			tgs.Res = xjson.NewError(res)
		}
		tgs.CurPhase = targetStepPhaseEnd
		tr.mu.Unlock()
		tr.cond.Signal()
		return err
		// Check for cancellation.
		// Notably we are not checking for the pause condition here:
		// when paused, we want to let all the injected targets to finish
		// and collect all the results they produce. If that doesn't happen,
		// step runner will close resCh on its way out and unlock us.
	case <-ctx.Done():
		tr.mu.Lock()
		log.Debugf("%s: canceled 2", tgs)
		tr.mu.Unlock()
		return statectx.ErrCanceled
	}
}

// targetHandler takes a single target through each step of the pipeline in sequence.
// It injects the target, waits for the result, then moves on to the next step.
func (tr *TestRunner) targetHandler(ctx, stepCtx statectx.Context, tgs *targetState) {
	log := logging.AddField(tr.log, "target", tgs.tgt.ID)
	log.Debugf("%s: target handler active", tgs)
	// NB: CurStep may be non-zero on entry if resumed
loop:
	for i := tgs.CurStep; i < len(tr.steps); {
		// Early check for pause or cancelation.
		select {
		case <-ctx.Paused():
			log.Debugf("%s: paused 0", tgs)
			break loop
		case <-ctx.Done():
			log.Debugf("%s: canceled 0", tgs)
			break loop
		default:
		}
		tr.mu.Lock()
		ss := tr.steps[i]
		if tgs.CurPhase == targetStepPhaseEnd {
			// This target already terminated.
			// Can happen if resumed from terminal state.
			tr.mu.Unlock()
			break loop
		}
		tgs.CurPhase = targetStepPhaseBegin
		tr.mu.Unlock()
		// Make sure we have a step runner active. If not, start one.
		tr.runStepIfNeeded(stepCtx, ss)
		// Inject the target.
		err := tr.injectTarget(ctx, tgs, ss, log)
		// Await result. It will be communicated to us by the step runner
		// and returned in tgs.res.
		if err == nil {
			err = tr.awaitTargetResult(ctx, tgs, ss, log)
		}
		tr.mu.Lock()
		if err != nil {
			ss.log.Errorf("%s", err)
			if err != statectx.ErrCanceled {
				ss.setErrLocked(err)
			} else {
				log.Debugf("%s: canceled 1", tgs)
			}
			tr.mu.Unlock()
			break
		}
		if tgs.Res != nil {
			tr.mu.Unlock()
			break
		}
		i++
		if i < len(tr.steps) {
			tgs.CurStep = i
			tgs.CurPhase = targetStepPhaseInit
		}
		tr.mu.Unlock()
	}
	tr.mu.Lock()
	log.Debugf("%s: target handler finished", tgs)
	tgs.resCh = nil
	tr.cond.Signal()
	tr.mu.Unlock()
}

// runStepIfNeeded starts the step runner goroutine if not already running.
func (tr *TestRunner) runStepIfNeeded(ctx statectx.Context, ss *stepState) {
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

func (ss *stepState) setErr(mu sync.Locker, err error) {
	mu.Lock()
	defer mu.Unlock()
	ss.setErrLocked(err)
}

// setErrLocked sets step runner error unless already set.
func (ss *stepState) setErrLocked(err error) {
	if err == nil || ss.runErr != nil {
		return
	}
	ss.log.Errorf("err: %v", err)
	ss.runErr = err
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
func (tr *TestRunner) stepRunner(ctx statectx.Context, ss *stepState) {
	ss.log.Debugf("%s: step runner active", ss)
	defer func() {
		if r := recover(); r != nil {
			tr.mu.Lock()
			ss.stepRunning = false
			ss.setErrLocked(&cerrors.ErrTestStepPaniced{
				StepName:   ss.sb.TestStepLabel,
				StackTrace: fmt.Sprintf("%s / %s", r, debug.Stack()),
			})
			tr.mu.Unlock()
			tr.safeCloseOutCh(ss)
		}
	}()
	chans := test.TestStepChannels{In: ss.inCh, Out: ss.outCh, Err: ss.errCh}
	runErr := ss.sb.TestStep.Run(ctx, chans, ss.sb.Parameters, ss.ev)
	ss.log.Debugf("%s: step runner finished %v", ss, runErr)
	tr.mu.Lock()
	ss.stepRunning = false
	ss.setErrLocked(runErr)
	tr.mu.Unlock()
	// Signal to the result processor that no more will be coming.
	tr.safeCloseOutCh(ss)
}

// reportTargetResult reports result of executing a step to the appropriate target handler.
func (tr *TestRunner) reportTargetResult(ctx statectx.Context, ss *stepState, tgt *target.Target, res error) error {
	resCh, err := func() (chan error, error) {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		tgs := tr.targets[tgt.ID]
		if tgs == nil {
			return nil, &cerrors.ErrTestStepReturnedUnexpectedResult{
				StepName: ss.sb.TestStepLabel,
				Target:   tgt.ID,
			}
		}
		if ss.tgtDone[tgt] {
			return nil, &cerrors.ErrTestStepReturnedDuplicateResult{
				StepName: ss.sb.TestStepLabel,
				Target:   tgt.ID,
			}
		}
		ss.tgtDone[tgt] = true
		// Begin is also allowed here because it may happen that we get a result before target handler updates phase.
		if tgs.CurStep != ss.stepIndex ||
			(tgs.CurPhase != targetStepPhaseBegin && tgs.CurPhase != targetStepPhaseRun) {
			return nil, &cerrors.ErrTestStepReturnedUnexpectedResult{
				StepName: ss.sb.TestStepLabel,
				Target:   tgt.ID,
			}
		}
		if tgs.resCh == nil {
			select {
			case <-ctx.Done():
				// If canceled, target handler may have left early. We don't care though.
				return nil, statectx.ErrCanceled
			default:
				// This should not happen, must be an internal error.
				return nil, fmt.Errorf("%s: target handler %s is not there, dropping result on the floor", ss, tgs)
			}
		}
		tgs.CurPhase = targetStepPhaseResultPending
		ss.log.Debugf("%s: result for %s: %v", ss, tgs, res)
		return tgs.resCh, nil
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

func (tr *TestRunner) safeCloseOutCh(ss *stepState) {
	defer func() {
		if r := recover(); r != nil {
			ss.setErr(&tr.mu, &cerrors.ErrTestStepClosedChannels{StepName: ss.sb.TestStepLabel})
		}
	}()
	close(ss.outCh)
}

// safeCloseErrCh closes error channel safely, even if it has already been closed.
func (tr *TestRunner) safeCloseErrCh(ss *stepState) {
	defer func() {
		if r := recover(); r != nil {
			ss.setErr(&tr.mu, &cerrors.ErrTestStepClosedChannels{StepName: ss.sb.TestStepLabel})
		}
	}()
	close(ss.errCh)
}

// stepReader receives results from the step's output channel and forwards them to the appropriate target handlers.
func (tr *TestRunner) stepReader(ctx statectx.Context, ss *stepState) {
	ss.log.Debugf("%s: step reader active", ss)
	var err error
	outCh := ss.outCh
	cancelCh := ctx.Done()
	var shutdownTimeoutCh <-chan time.Time
loop:
	for {
		select {
		case tgt, ok := <-outCh:
			if !ok {
				ss.log.Debugf("%s: out chan closed", ss)
				// At this point we may still have an error to report,
				// wait until error channel is emptied too.
				outCh = nil
				tr.safeCloseErrCh(ss)
				continue loop
			}
			if err = tr.reportTargetResult(ctx, ss, tgt, nil); err != nil {
				break loop
			}
		case res, ok := <-ss.errCh:
			if !ok {
				tr.mu.Lock()
				if ss.stepRunning {
					// Error channel is always closed after the output,
					// which is only closed after runner goroutine has exited.
					// If we got here, it means that the plugin closed the error channel.
					ss.setErrLocked(&cerrors.ErrTestStepClosedChannels{StepName: ss.sb.TestStepLabel})
				}
				tr.mu.Unlock()
				ss.log.Debugf("%s: err chan closed", ss)
				break loop
			}
			if err = tr.reportTargetResult(ctx, ss, res.Target, res.Err); err != nil {
				break loop
			}
		case <-cancelCh:
			ss.log.Debugf("%s: canceled 3, draining", ss)
			// Allow some time to drain
			cancelCh = nil
			shutdownTimeoutCh = time.After(tr.shutdownTimeout)
		case <-shutdownTimeoutCh:
			ss.setErr(&tr.mu, &cerrors.ErrTestStepsNeverReturned{})
		}
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	ss.setErrLocked(err)
	if ss.stepRunning && ss.runErr == nil {
		// This means that plugin closed its channels before leaving.
		ss.setErrLocked(&cerrors.ErrTestStepClosedChannels{StepName: ss.sb.TestStepLabel})
	}
	ss.readerRunning = false
	ss.log.Debugf("%s: step reader finished, %t %t %v", ss, ss.stepRunning, ss.readerRunning, ss.runErr)
	tr.cond.Signal()
}

// checkStepRunnersLocked checks if any step runner has encountered an error.
func (tr *TestRunner) checkStepRunners() error {
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
func (tr *TestRunner) runMonitor() error {
	tr.log.Debugf("monitor: active")
	tr.mu.Lock()
	defer tr.mu.Unlock()
	// First, compute the starting step of the pipeline (it may be non-zero
	// if the pipeline was resumed).
	minStep := len(tr.steps)
	for _, tgs := range tr.targets {
		if tgs.CurStep < minStep {
			minStep = tgs.CurStep
		}
	}
	if minStep < len(tr.steps) {
		tr.log.Debugf("monitor: starting at step %s", tr.steps[minStep])
	}

	// Run the main loop.
	pass := 1
	var runErr error
stepLoop:
	for step := minStep; step < len(tr.steps); pass++ {
		ss := tr.steps[step]
		tr.log.Debugf("monitor pass %d: current step %s", pass, ss)
		// Check if all the targets have either made it past the injection phase or terminated.
		ok := true
		for _, tgs := range tr.targets {
			tr.log.Debugf("monitor pass %d: %s: %s", pass, ss, tgs)
			if tgs.resCh == nil { // Not running anymore
				continue
			}
			if tgs.CurStep < step || tgs.CurPhase < targetStepPhaseRun {
				tr.log.Debugf("monitor pass %d: %s: not all targets injected yet (%s)", pass, ss, tgs)
				ok = false
				break
			}
		}
		if runErr = tr.checkStepRunners(); runErr != nil {
			break stepLoop
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
	// Wait for all the targets to finish.
	tr.log.Debugf("monitor: waiting for targets to finish")
tgtLoop:
	for ; runErr == nil; pass++ {
		ok := true
		for _, tgs := range tr.targets {
			tr.log.Debugf("monitor pass %d: %s", pass, tgs)
			if runErr = tr.checkStepRunners(); runErr != nil {
				break tgtLoop
			}
			if tgs.resCh != nil && tgs.CurStep < len(tr.steps) {
				ss := tr.steps[tgs.CurStep]
				if tgs.CurPhase == targetStepPhaseRun && !ss.readerRunning {
					// Target has been injected but step runner has exited, this target has been lost.
					runErr = &cerrors.ErrTestStepLostTargets{
						StepName: ss.sb.TestStepLabel,
						Targets:  []string{tgs.tgt.ID},
					}
					break tgtLoop
				}
				ok = false
				break
			}
		}
		if ok {
			break
		}
		// Wait for notification: as progress is being made, we get notified.
		tr.cond.Wait()
	}
	tr.log.Debugf("monitor: finished, %v", runErr)
	return runErr
}

func NewTestRunnerWithTimeouts(shutdownTimeout time.Duration) *TestRunner {
	tr := &TestRunner{
		shutdownTimeout: shutdownTimeout,
	}
	tr.cond = sync.NewCond(&tr.mu)
	return tr
}

func NewTestRunner() *TestRunner {
	return NewTestRunnerWithTimeouts(config.TestRunnerShutdownTimeout)
}

func (tph targetStepPhase) String() string {
	switch tph {
	case targetStepPhaseInvalid:
		return "INVALID"
	case targetStepPhaseInit:
		return "init"
	case targetStepPhaseBegin:
		return "begin"
	case targetStepPhaseRun:
		return "run"
	case targetStepPhaseResultPending:
		return "result_pending"
	case targetStepPhaseEnd:
		return "end"
	}
	return fmt.Sprintf("???(%d)", tph)
}

func (ss *stepState) String() string {
	return fmt.Sprintf("[#%d %s]", ss.stepIndex, ss.sb.TestStepLabel)
}

func (tgs *targetState) String() string {
	var resText string
	if tgs.Res != nil {
		resStr := fmt.Sprintf("%v", tgs.Res)
		if len(resStr) > 20 {
			resStr = resStr[:20] + "..."
		}
		resText = fmt.Sprintf("%q", resStr)
	} else {
		resText = "<nil>"
	}
	finished := tgs.resCh == nil
	return fmt.Sprintf("[%s %d %s %t %s]",
		tgs.tgt, tgs.CurStep, tgs.CurPhase, finished, resText)
}
