// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"container/list"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/sirupsen/logrus"
)

// TestRunnerTimeouts collects all the timeouts values that the test runner uses
type TestRunnerTimeouts struct {
	StepInjectTimeout   time.Duration
	MessageTimeout      time.Duration
	ShutdownTimeout     time.Duration
	StepShutdownTimeout time.Duration
}

// routingCh represents a set of unidirectional channels used by the routing subsystem.
// There is a routing block for each TestStep of the pipeline, which is responsible for
// the following actions:
// * Targets in egress from the previous routing block are injected into the
// current TestStep
// * Targets in egress from the current TestStep are injected into the next
// routing block
type routingCh struct {
	// routeIn and routeOut connect the routing block to other routing blocks
	routeIn  <-chan *target.Target
	routeOut chan<- *target.Target
	// Channels that connect the routing block to the TestStep
	stepIn  chan<- *target.Target
	stepOut <-chan *target.Target
	stepErr <-chan cerrors.TargetError
	// targetErr connects the routing block directly to the TestRunner. Failing
	// targets are acquired by the TestRunner via this channel
	targetErr chan<- cerrors.TargetError
}

// stepCh represents a set of bidirectional channels that a TestStep and its associated
// routing block use to communicate. The TestRunner forces the direction of each
// channel when connecting the TestStep to the routing block.
type stepCh struct {
	stepIn  chan *target.Target
	stepOut chan *target.Target
	stepErr chan cerrors.TargetError
}

type injectionCh struct {
	stepIn   chan<- *target.Target
	resultCh chan<- injectionResult
}

// injectionResult represents the result of an injection goroutine
type injectionResult struct {
	target *target.Target
	err    error
}

// routeResult represents the result of routing block, possibly carrying error information
type routeResult struct {
	bundle test.TestStepBundle
	err    error
}

// stepResult represents the result of a TestStep, possibly carrying error information
type stepResult struct {
	jobID  types.JobID
	runID  types.RunID
	bundle test.TestStepBundle
	err    error
}

// pipelineCtrlCh represents multiple result and control channels that the pipeline uses
// to collect results from routing blocks, steps and target completing the test and to
//  signa cancellation to various pipeline subsystems
type pipelineCtrlCh struct {
	routingResultCh <-chan routeResult
	stepResultCh    <-chan stepResult
	targetOut       <-chan *target.Target
	targetErr       <-chan cerrors.TargetError
	// cancelRouting is a control channel used to cancel routing blocks in the pipeline
	cancelRoutingCh chan struct{}
	// cancelStep is a control channel used to cancel the steps of the pipeline
	cancelStepsCh chan struct{}
	// pauseSteps is a control channel used to pause the steps of the pipeline
	pauseStepsCh chan struct{}
}

// TestRunner is the main runner of TestSteps in ConTest. `results` collects
// the results of the run. It is not safe to access `results` concurrently.
type TestRunner struct {
	timeouts TestRunnerTimeouts
}

// targetWriter is a helper object which exposes methods to write targets into step channels
type targetWriter struct {
	log      *logrus.Entry
	timeouts TestRunnerTimeouts
}

func (w *targetWriter) writeTimeout(terminate <-chan struct{}, ch chan<- *target.Target, target *target.Target, timeout time.Duration) error {
	w.log.Debugf("writing target %+v, timeout %v", target, timeout)
	start := time.Now()
	select {
	case <-terminate:
		w.log.Debugf("terminate requested while writing target %+v", target)
	case ch <- target:
	case <-time.After(timeout):
		return fmt.Errorf("timeout (%v) while writing target %+v", timeout, target)
	}
	w.log.Debugf("done writing target %+v, spent %v", target, time.Since(start))
	return nil
}

// writeTargetWithResult attempts to deliver a Target on the input channel of a step,
// returning the result of the operation on the result channel wrapped in the
// injectionCh argument
func (w *targetWriter) writeTargetWithResult(terminate <-chan struct{}, target *target.Target, ch injectionCh, wg *sync.WaitGroup) {
	defer wg.Done()
	err := w.writeTimeout(terminate, ch.stepIn, target, w.timeouts.StepInjectTimeout)
	select {
	case <-terminate:
	case ch.resultCh <- injectionResult{target: target, err: err}:
	case <-time.After(w.timeouts.MessageTimeout):
		w.log.Panicf("timeout while writing result for target %+v after %v", target, w.timeouts.MessageTimeout)
	}
}

// writeTargetError writes a TargetError object to a TargetError channel with timeout
func (w *targetWriter) writeTargetError(terminate <-chan struct{}, ch chan<- cerrors.TargetError, targetError cerrors.TargetError, timeout time.Duration) error {
	select {
	case <-terminate:
	case ch <- targetError:
	case <-time.After(timeout):
		return fmt.Errorf("timeout while writing targetError %+v", targetError)
	}
	return nil
}

func newTargetWriter(log *logrus.Entry, timeouts TestRunnerTimeouts) *targetWriter {
	return &targetWriter{log: log, timeouts: timeouts}
}

// pipeline represents a sequence of steps through which targets flow. A pipeline could implement
// either a test sequence or a cleanup sequence
type pipeline struct {
	log *logrus.Entry

	bundles []test.TestStepBundle
	targets []*target.Target

	jobID types.JobID
	runID types.RunID

	state *State
	t     *test.Test

	timeouts TestRunnerTimeouts

	// ctrlChannels represents a set of result and completion channels for this pipeline,
	// used to collect the results of routing blocks, steps and targets completing
	// the pipeline. It's available only after having initialized the pipeline
	ctrlChannels *pipelineCtrlCh
}

// Route implements the routing block associated with a TestStep that routes Targets
// into and out of a TestStep. It performs the following actions:
// * Consumes targets in input from the previous routing block
// * Asynchronously injects targets into the associated TestStep
// * Consumes targets in output from the associated TestStep
// * Asynchronously forwards targets to the following routing block
func (p *pipeline) route(terminateRoute <-chan struct{}, bundle test.TestStepBundle, routingCh routingCh, resultCh chan<- routeResult, ev testevent.EmitterFetcher) {

	stepLabel := bundle.TestStepLabel
	log := logging.AddField(p.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "route")

	log.Debugf("initializing routing for %s", stepLabel)
	targetWriter := newTargetWriter(log, p.timeouts)

	terminate := make(chan struct{})

	tRouteIn := routingCh.routeIn
	tStepOut := routingCh.stepOut
	tStepErr := routingCh.stepErr

	// Channel that the injection goroutine uses to communicate back with the
	// main routing logic
	injectResultCh := make(chan injectionResult)
	injectionChannels := injectionCh{stepIn: routingCh.stepIn, resultCh: injectResultCh}

	// `targets` is used to buffer targets coming from the previous routing blocks,
	// queueing them for injection into the TestStep. The list is accessed
	// synchronously by a single goroutine.
	targets := list.New()

	// `ingressTarget` and `egressTarget` are used to keep track of ingress and
	// egress times and perform sanity checks on the input/output of the TestStep
	ingressTarget := make(map[*target.Target]time.Time)
	egressTarget := make(map[*target.Target]time.Time)

	var (
		err           error
		stepInClosed  bool
		pendingTarget *target.Target
		injectionWg   sync.WaitGroup
	)

	for {
		log.Debugf("listening")
		select {
		case <-terminateRoute:
			err = fmt.Errorf("termination requested for routing into %s", stepLabel)
			break
		case injectionResult := <-injectResultCh:
			log.Debugf("received injection result for %v", injectionResult.target)
			ingressTarget[pendingTarget] = time.Now()
			pendingTarget = nil
			if injectionResult.err != nil {
				err = fmt.Errorf("routing failed while injecting target %+v into %s", injectionResult.err, stepLabel)
				targetInErrEv := testevent.Data{EventName: target.EventTargetInErr, Target: injectionResult.target}
				if err := ev.Emit(targetInErrEv); err != nil {
					log.Warningf("could not emit %v event for target: %+v", targetInErrEv, *injectionResult.target)
				}
				break
			}
			targetInEv := testevent.Data{EventName: target.EventTargetIn, Target: injectionResult.target}
			if err := ev.Emit(targetInEv); err != nil {
				log.Warningf("could not emit %v event for Target: %+v", targetInEv, *injectionResult.target)
			}
			if targets.Len() > 0 {
				pendingTarget = targets.Back().Value.(*target.Target)
				targets.Remove(targets.Back())
				injectionWg.Add(1)
				log.Debugf("writing target %v into test step", pendingTarget)
				go targetWriter.writeTargetWithResult(terminate, pendingTarget, injectionChannels, &injectionWg)
			}
		case t, chanIsOpen := <-tRouteIn:
			if !chanIsOpen {
				// The previous routing block has closed our input channel, signaling that
				// no more Targets will come through. Block reading from this channel
				log.Debugf("routing input channel closed")
				tRouteIn = nil
			} else {
				// Buffer the target and check if there is already an injection in progress.
				// If so, pending targets will be dequeued only at the next result available
				// on `injectResultCh`.
				log.Debugf("received target %v in input", t)
				targets.PushFront(t)
				if pendingTarget == nil {
					pendingTarget = targets.Back().Value.(*target.Target)
					targets.Remove(targets.Back())
					injectionWg.Add(1)
					log.Debugf("writing target %v into test step", pendingTarget)
					go targetWriter.writeTargetWithResult(terminate, pendingTarget, injectionChannels, &injectionWg)
				}
			}
		case t, chanIsOpen := <-tStepOut:
			if !chanIsOpen {
				tStepOut = nil
			} else {
				if _, targetPresent := egressTarget[t]; targetPresent {
					err = fmt.Errorf("step %s returned target %+v multiple times", bundle.TestStepLabel, t)
					break
				}
				// Emit an event signaling that the target has left the TestStep
				targetOutEv := testevent.Data{EventName: target.EventTargetOut, Target: t}
				if err := ev.Emit(targetOutEv); err != nil {
					log.Warningf("could not emit %v event for target: %v", targetOutEv, *t)
				}
				// Register egress time and forward target to the next routing block
				egressTarget[t] = time.Now()
				if err := targetWriter.writeTimeout(terminateRoute, routingCh.routeOut, t, p.timeouts.MessageTimeout); err != nil {
					log.Panicf("could not forward target to the test runner: %+v", err)
				}
			}
		case targetError, chanIsOpen := <-tStepErr:
			if !chanIsOpen {
				tStepErr = nil
			} else {
				if _, targetPresent := egressTarget[targetError.Target]; targetPresent {
					err = fmt.Errorf("step %s returned target %+v multiple times", bundle.TestStepLabel, targetError.Target)
					break
				}
				// Emit an event signaling that the target has left the TestStep with an error
				targetErrPayload := target.ErrPayload{Error: targetError.Err.Error()}
				payloadEncoded, err := json.Marshal(targetErrPayload)
				if err != nil {
					log.Warningf("could not encode target error ('%s'): %v", targetErrPayload, err)
				}

				rawPayload := json.RawMessage(payloadEncoded)

				targetErrEv := testevent.Data{EventName: target.EventTargetErr, Target: targetError.Target, Payload: &rawPayload}
				if err := ev.Emit(targetErrEv); err != nil {
					log.Warningf("could not emit %v event for target: %v", targetErrEv, *targetError.Target)
				}
				// Register egress time and forward the failing target to the TestRunner
				egressTarget[targetError.Target] = time.Now()
				if err := targetWriter.writeTargetError(terminateRoute, routingCh.targetErr, targetError, p.timeouts.MessageTimeout); err != nil {
					log.Panicf("could not forward target (%+v) to the test runner: %v", targetError.Target, err)
				}
			}
		} // end of select statement

		if err != nil {
			break
		}
		if tStepErr == nil && tStepOut == nil {
			// If the TestStep has closed its out and err channels in compliance with
			// ConTest API, it means that target injection has completed and we have already
			// closed the TestStep input channel. routingCh.routeOut is closed when `Route`
			// terminates.
			break
		}
		if targets.Len() == 0 && tRouteIn == nil && pendingTarget == nil && !stepInClosed {
			// If we have already acquired and injected all targets, signal to the TestStep
			// that no more targets will come through by closing the input channel.
			// Note that the input channel is not closed if routing is cancelled.
			// A TestStep is expected to always be reactive to cancellation even when
			// acquiring targets from the input channel.
			stepInClosed = true
			close(routingCh.stepIn)
		}
	}

	// If we are not handling an error condition, check if all the targets that we
	// have injected have been returned by the TestStep. If that condition is met,
	// we might have been asked to terminate via cancellation signal, or we might
	// have terminated successfully. We close routing output channel to signal to
	// the next routing block that no more targets will come through only in the latter
	// case.
	if err == nil {
		if len(ingressTarget) != len(egressTarget) {
			err = fmt.Errorf("step %s completed but did not return all injected Targets", bundle.TestStepLabel)
		} else {
			select {
			case <-terminateRoute:
			default:
				close(routingCh.routeOut)
			}
		}
	}

	// Signal termination to the injection routines regardless of the result of the
	// routing. If the routing completed successfully, this is a no-op. If there is an
	// injection goroutine running, wait for it to terminate, as we might have gotten
	// here after a cancellation signal.
	close(terminate)
	injectionWg.Wait()

	// Send the result to the test runner, which is guaranteed to be listening. If
	// the TestRunner is not responsive before `MessageTimeout`, we are either running
	// on a system under heavy load which makes the runtime unable to properly schedule
	// goroutines, or we are hitting a bug.
	select {
	case resultCh <- routeResult{bundle: bundle, err: err}:
	case <-time.After(p.timeouts.MessageTimeout):
		log.Panicf("could not send routing block result")
	}
}

// runStep runs synchronously a TestStep and peforms sanity checks on the status
// of the input/output channels on the defer control path. When the TestStep returns,
// the associated output channels are closed. This signals to the routing subsytem
// that this step of the pipeline will not be producing any more targets. The API
// dictates that the TestStep can only return if its input channel has been closed,
// which indicates that no more targets will be submitted to that step. If the
// TestStep does not comply with this rule, an error is sent downstream to the pipeline
// runner, which will in turn shutdown the whole pipeline and flag the TestStep
// as misbehaving. All attempts to send downstream a result after a TestStep complete
// are protected by a timeout and fail open. This because it's not guaranteed that
// the TestRunner will still be listening on the result channels. If the TestStep hangs
// indefinitely and does not respond to cancellation signals, the TestRunner will
// flag it as misbehaving and return. If the TestStep returns once the TestRunner
// has completed, it will timeout trying to write on the result channel.
func (p *pipeline) runStep(cancel, pause <-chan struct{}, jobID types.JobID, runID types.RunID, bundle test.TestStepBundle, stepCh stepCh, resultCh chan<- stepResult, ev testevent.EmitterFetcher) {

	stepLabel := bundle.TestStepLabel
	log := logging.AddField(p.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "runStep")

	log.Debugf("initializing step")
	timeout := p.timeouts.MessageTimeout
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("step %s paniced (%v): %s", stepLabel, r, debug.Stack())
			select {
			case resultCh <- stepResult{jobID: jobID, runID: runID, bundle: bundle, err: err}:
			case <-time.After(p.timeouts.MessageTimeout):
				log.Warningf("sending error back from test step runner timed out after %v: %v", timeout, err)
				return
			}
			return
		}
	}()

	// We should not run a test step if there're no targets in stepCh.stepIn
	// First we check if there's at least one incoming target, and only
	// then we call `bundle.TestStep.Run()`.
	//
	// ITS: https://github.com/facebookincubator/contest/issues/101
	stepIn, onFirstTargetChan, onNoTargetsChan := waitForFirstTarget(stepCh.stepIn, cancel, pause)

	haveTargets := false
	select {
	case <-onFirstTargetChan:
		log.Debugf("first target received, will start step")
		haveTargets = true
	case <-cancel:
		log.Debugf("cancelled")
	case <-pause:
		log.Debugf("paused")
	case <-onNoTargetsChan:
		log.Debugf("no targets")
	}

	var err error
	if haveTargets {
		// Run the TestStep and defer to the return path the handling of panic
		// conditions. If multiple error conditions occur, send downstream only
		// the first error encountered.
		channels := test.TestStepChannels{
			In:  stepIn,
			Out: stepCh.stepOut,
			Err: stepCh.stepErr,
		}
		err = bundle.TestStep.Run(cancel, pause, channels, bundle.Parameters, ev)
	}

	log.Debugf("step %s returned", bundle.TestStepLabel)
	var (
		cancellationAsserted bool
		pauseAsserted        bool
	)
	// Check if we are shutting down. If so, do not perform sanity checks on the
	// channels but return immediately as the TestStep itself probably returned
	// because it honored the termination signal.
	select {
	case <-cancel:
		log.Debugf("cancelled")
		cancellationAsserted = true
		if err == nil {
			err = fmt.Errorf("test step cancelled")
		}
	case <-pause:
		log.Debugf("paused")
		pauseAsserted = true
	default:
	}

	if cancellationAsserted || pauseAsserted {
		select {
		case resultCh <- stepResult{jobID: jobID, runID: runID, bundle: bundle, err: err}:
		case <-time.After(timeout):
			log.Warningf("sending error back from TestStep runner timed out after %v: %v", timeout, err)
		}
		return
	}

	// If the TestStep has crashed with an error, return immediately the result to the TestRunner
	// which in turn will issue a cancellation signal to the pipeline
	if err != nil {
		select {
		case resultCh <- stepResult{jobID: jobID, runID: runID, bundle: bundle, err: err}:
		case <-time.After(timeout):
			log.Warningf("sending error back from test step runner (%s) timed out after %v: %v", stepLabel, timeout, err)
		}
		return
	}

	// Perform sanity checks on the status of the channels. The TestStep API
	// mandates that output and error channels shall not be closed by the
	// TestStep itself. If the TestStep does not comply with the API, it is
	// flagged as misbehaving.
	select {
	case _, ok := <-stepCh.stepIn:
		if !ok {
			break
		}
		// stepCh.stepIn is not closed, but the TestStep returned, which is a violation
		// of the API. Record the error if no other error condition has been seen.
		if err == nil {
			err = fmt.Errorf("step %s returned, but input channel is not closed (api violation; case 0)", stepLabel)
		}
	default:
		// stepCh.stepIn is not closed, and a read operation would block. The TestStep
		// does not comply with the API (see above).
		if err == nil {
			err = fmt.Errorf("step %s returned, but input channel is not closed (api violation; case 1)", stepLabel)
		}
	}

	select {
	case _, ok := <-stepCh.stepOut:
		if !ok {
			// stepOutCh has been closed. This is a violation of the API. Record the error
			// if no other error condition has been seen.
			if err == nil {
				err = &cerrors.ErrTestStepClosedChannels{StepName: stepLabel}
			}
		}
	default:
		// stepCh.stepOut is open. Flag it for closure to signal to the routing subsystem that
		// no more Targets will go through from this channel.
		close(stepCh.stepOut)
	}

	select {
	case _, ok := <-stepCh.stepErr:
		if !ok {
			// stepErrCh has been closed. This is a violation of the API. Record the error
			// if no other error condition has been seen.
			if err == nil {
				err = &cerrors.ErrTestStepClosedChannels{StepName: stepLabel}
			}
		}
	default:
		// stepCh.stepErr is open. Flag it for closure to signal to the routing subsystem that
		// no more Targets will go through from this channel.
		close(stepCh.stepErr)
	}

	select {
	case resultCh <- stepResult{jobID: jobID, runID: runID, bundle: bundle, err: err}:
	case <-time.After(timeout):
		log.Warningf("sending error back from step runner (%s) timed out after %v: %v", stepLabel, timeout, err)
		return
	}
}

// waitTargets reads results coming from results channels until all Targets
// have completed or an error occurs. If all Targets complete successfully, it checks
// whether TestSteps and routing blocks have completed as well. If not, returns an
// error. Termination is signalled via terminate channel.
func (p *pipeline) waitTargets(terminate <-chan struct{}, ch *pipelineCtrlCh, completedCh chan<- *target.Target) error {

	log := logging.AddField(p.log, "phase", "waitTargets")

	var (
		err                  error
		completedTarget      *target.Target
		completedTargetError error
	)

	writer := newTargetWriter(log, p.timeouts)
	for {

		if completedTarget != nil {
			p.state.SetTarget(completedTarget, completedTargetError)
			log.Debugf("writing target %+v on the completed channel", completedTarget)
			if err := writer.writeTimeout(terminate, completedCh, completedTarget, p.timeouts.MessageTimeout); err != nil {
				log.Panicf("could not write completed target: %v", err)
			}
			completedTarget = nil
			completedTargetError = nil
		}

		if err != nil {
			return err
		}

		if len(p.state.CompletedTargets()) == len(p.targets) {
			log.Debugf("all targets completed")
			break
		}

		select {
		case <-terminate:
			// When termination is signaled just stop wait. It is up
			// to the caller to decide how to further handle pipeline termination.
			log.Debugf("termination requested")
			return nil
		case res := <-ch.routingResultCh:
			err = res.err
			p.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-ch.stepResultCh:
			err = res.err
			if err != nil {
				payload, jmErr := json.Marshal(err.Error())
				if jmErr != nil {
					log.Warningf("failed to marshal error string to JSON: %v", jmErr)
					continue
				}
				rm := json.RawMessage(payload)
				ev := storage.NewTestEventEmitterFetcher(testevent.Header{
					JobID:         res.jobID,
					RunID:         res.runID,
					TestName:      p.t.Name,
					TestStepLabel: res.bundle.TestStepLabel,
				})
				errEv := testevent.Data{
					EventName: EventTestError,
					// this event is not associated to any target, e.g. a plugin has
					// returned an error.
					Target:  nil,
					Payload: &rm,
				}
				// emit test event containing the completion error
				if err := ev.Emit(errEv); err != nil {
					log.Warningf("could not emit completion error event %v", errEv)
				}
			}
			p.state.SetStep(res.bundle.TestStepLabel, res.err)

		case targetErr := <-ch.targetErr:
			completedTarget = targetErr.Target
			completedTargetError = targetErr.Err
		case target, chanIsOpen := <-ch.targetOut:
			if !chanIsOpen {
				if len(p.state.CompletedTargets()) != len(p.targets) {
					err = fmt.Errorf("not all targets completed, but output channel is closed")
				}
			}
			completedTarget = target
		}
	}

	// The test run completed, we have collected all Targets. TestSteps might have already
	// closed `ch.out`, in which case the pipeline terminated correctly (channels are closed
	// in a "domino" sequence, so seeing the last channel closed indicates that the
	// sequence of close operations has completed). If `ch.out` is still open,
	// there are still TestSteps that might have not returned. Wait for all
	// TestSteps to complete or `StepShutdownTimeout` to occur.
	log.Infof("waiting for all steps to complete")
	return p.waitSteps(ch)
}

// waitTermination reads results coming from result channels waiting
// for the pipeline to completely shutdown before `ShutdownTimeout` occurs. A
// "complete shutdown" means that all TestSteps and routing blocks have sent
// downstream their results.
func (p *pipeline) waitTermination() error {

	log := logging.AddField(p.log, "phase", "waitTermination")
	log.Printf("waiting for pipeline to terminate")

	for {
		select {
		case <-time.After(p.timeouts.ShutdownTimeout):
			incompleteSteps := p.state.IncompleteSteps(p.bundles)
			if len(incompleteSteps) > 0 {
				return &cerrors.ErrTestStepsNeverReturned{StepNames: p.state.IncompleteSteps(p.bundles)}
			}
			return fmt.Errorf("pipeline did not return but all test steps completed")
		case res := <-p.ctrlChannels.routingResultCh:
			p.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-p.ctrlChannels.stepResultCh:
			p.state.SetStep(res.bundle.TestStepLabel, res.err)
		}

		stepsCompleted := len(p.state.CompletedSteps()) == len(p.bundles)
		routingCompleted := len(p.state.CompletedRouting()) == len(p.bundles)
		if stepsCompleted && routingCompleted {
			return nil
		}
	}
}

// waitSteps reads results coming from result channels until `StepShutdownTimeout`
// occurs or an error is encountered. It then checks whether TestSteps and routing
// blocks have all returned correctly. If not, it returns an error.
func (p *pipeline) waitSteps(ch *pipelineCtrlCh) error {

	log := logging.AddField(p.log, "phase", "waitSteps")

	var err error

wait_test_step:
	for {
		select {
		case <-time.After(p.timeouts.StepShutdownTimeout):
			log.Debugf("timed out waiting for steps to complete after %v", p.timeouts.StepShutdownTimeout)
			break wait_test_step
		case res := <-ch.routingResultCh:
			log.Debugf("received result for %s", res.bundle.TestStepLabel)
			err = res.err
			p.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-ch.stepResultCh:
			err = res.err
			p.state.SetStep(res.bundle.TestStepLabel, res.err)
		}

		if err != nil {
			return err
		}
		stepsCompleted := len(p.state.CompletedSteps()) == len(p.bundles)
		routingCompleted := len(p.state.CompletedRouting()) == len(p.bundles)
		if stepsCompleted && routingCompleted {
			break wait_test_step
		}
	}

	incompleteSteps := p.state.IncompleteSteps(p.bundles)
	if len(incompleteSteps) > 0 {
		err = &cerrors.ErrTestStepsNeverReturned{StepNames: incompleteSteps}
	} else if len(p.state.CompletedRouting()) != len(p.bundles) {
		err = fmt.Errorf("not all routing completed")
	}
	return err
}

// init initializes the pipeline by connecting steps and routing blocks. The result of pipeline
// initialization is a set of control/result channels assigned to the pipeline object. The pipeline
// input channel is returned.
func (p *pipeline) init(cancel, pause <-chan struct{}) (routeInFirst chan *target.Target) {
	p.log.Debugf("starting")

	if p.ctrlChannels != nil {
		p.log.Panicf("pipeline is already initialized, control channel are already configured")
	}

	var (
		routeOut chan *target.Target
		routeIn  chan *target.Target
	)

	// termination channels are used to signal termination to injection and routing
	routingCancelCh := make(chan struct{})
	stepsCancelCh := make(chan struct{})
	stepsPauseCh := make(chan struct{})

	// result channels used to communicate result information from the routing blocks
	// and step executors
	routingResultCh := make(chan routeResult)
	stepResultCh := make(chan stepResult)
	targetErrCh := make(chan cerrors.TargetError)

	routeIn = make(chan *target.Target)
	for r, testStepBundle := range p.bundles {

		// Input and output channels for the TestStep
		stepInCh := make(chan *target.Target)
		stepOutCh := make(chan *target.Target)
		stepErrCh := make(chan cerrors.TargetError)

		routeOut = make(chan *target.Target)

		// First step of the pipeline, keep track of the routeIn channel as this is
		// going to be used to route channels into the pipeline from outside
		if r == 0 {
			routeInFirst = routeIn
		}

		stepChannels := stepCh{stepIn: stepInCh, stepErr: stepErrCh, stepOut: stepOutCh}
		routingChannels := routingCh{
			routeIn:   routeIn,
			routeOut:  routeOut,
			stepIn:    stepInCh,
			stepErr:   stepErrCh,
			stepOut:   stepOutCh,
			targetErr: targetErrCh,
		}

		// Build the Header that the the TestStep will be using for emitting events
		Header := testevent.Header{
			JobID:         p.jobID,
			RunID:         p.runID,
			TestName:      p.t.Name,
			TestStepLabel: testStepBundle.TestStepLabel,
		}
		ev := storage.NewTestEventEmitterFetcher(Header)
		go p.route(routingCancelCh, testStepBundle, routingChannels, routingResultCh, ev)
		go p.runStep(stepsCancelCh, stepsPauseCh, p.jobID, p.runID, testStepBundle, stepChannels, stepResultCh, ev)
		// The input of the next routing block is the output of the current routing block
		routeIn = routeOut
	}

	p.ctrlChannels = &pipelineCtrlCh{
		routingResultCh: routingResultCh,
		stepResultCh:    stepResultCh,
		targetErr:       targetErrCh,
		targetOut:       routeOut,

		cancelRoutingCh: routingCancelCh,
		cancelStepsCh:   stepsCancelCh,
		pauseStepsCh:    stepsPauseCh,
	}

	return
}

// run is a blocking method which executes the pipeline until successful or failed termination
func (p *pipeline) run(cancel, pause <-chan struct{}, completedTargets chan<- *target.Target) error {

	p.log.Debugf("run")
	if p.ctrlChannels == nil {
		p.log.Panicf("pipeline is not initialized, control channels are not available")
	}

	// Wait for the pipeline to complete. If an error occurrs, cancel all TestSteps
	// and routing blocks and wait again for completion until shutdown timeout occurrs.
	p.log.Infof("waiting for pipeline to complete")

	var (
		completionError      error
		terminationError     error
		cancellationAsserted bool
		pauseAsserted        bool
	)

	cancelWaitTargetsCh := make(chan struct{})
	// errCh collects errors coming from the routines which wait for the Test to complete
	errCh := make(chan error)
	go func() {
		errCh <- p.waitTargets(cancelWaitTargetsCh, p.ctrlChannels, completedTargets)
	}()

	select {
	case completionError = <-errCh:
	case <-cancel:
		close(cancelWaitTargetsCh)
		cancellationAsserted = true
		completionError = <-errCh
	case <-pause:
		close(cancelWaitTargetsCh)
		pauseAsserted = true
		completionError = <-errCh
	}

	if completionError != nil || cancellationAsserted {
		// If the Test has encountered an error or cancellation has been asserted,
		// terminate routing and and propagate the cancel signal to the steps
		cancellationAsserted = true
		if completionError != nil {
			p.log.Warningf("test failed to complete: %v. Forcing cancellation.", completionError)
		} else {
			p.log.Infof("cancellation was asserted")
		}
		close(p.ctrlChannels.cancelStepsCh)
		close(p.ctrlChannels.cancelRoutingCh)
	}

	if pauseAsserted {
		// If pause signal has been asserted, terminate routing and propagate the pause signal to the steps.
		p.log.Warningf("received pause request")
		close(p.ctrlChannels.pauseStepsCh)
		close(p.ctrlChannels.cancelRoutingCh)
	}

	// If either cancellation or pause have been asserted, we need to wait for the
	// pipeline to terminate
	if cancellationAsserted || pauseAsserted {
		go func() {
			errCh <- p.waitTermination()
		}()
		signal := "cancellation"
		if pauseAsserted {
			signal = "pause"
		}
		terminationError = <-errCh
		if terminationError != nil {
			p.log.Infof("test did not terminate correctly after %s signal: %v", signal, terminationError)
		} else {
			p.log.Infof("test terminated correctly after %s signal", signal)
		}
	} else {
		p.log.Infof("completed")
	}

	if completionError != nil {
		return completionError
	}
	return terminationError

}

func newPipeline(log *logrus.Entry, bundles []test.TestStepBundle, t *test.Test, targets []*target.Target, jobID types.JobID, runID types.RunID, timeouts TestRunnerTimeouts) *pipeline {
	p := pipeline{log: log, bundles: bundles, targets: targets, jobID: jobID, runID: runID, t: t, timeouts: timeouts}
	p.state = NewState()
	return &p
}

// Run implements the main logic of the TestRunner, i.e. the instantiation and
// connection of the TestSteps, routing blocks and pipeline runner.
func (tr *TestRunner) Run(cancel, pause <-chan struct{}, t *test.Test, targets []*target.Target, jobID types.JobID, runID types.RunID) error {

	if len(t.TestStepsBundles) == 0 {
		return fmt.Errorf("no steps to run for test")
	}

	// rootLog is propagated to all the subsystems of the pipeline
	rootLog := logging.GetLogger("pkg/runner")
	fields := make(map[string]interface{})
	fields["jobid"] = jobID
	fields["runid"] = runID
	rootLog = logging.AddFields(rootLog, fields)

	log := logging.AddField(rootLog, "phase", "run")
	pipeline := newPipeline(logging.AddField(rootLog, "entity", "pipeline"), t.TestStepsBundles, t, targets, jobID, runID, tr.timeouts)

	log.Infof("setting up pipeline")
	completedTargets := make(chan *target.Target)
	inCh := pipeline.init(cancel, pause)

	// inject targets in the step
	terminateInjectionCh := make(chan struct{})
	go func(terminate <-chan struct{}, inputChannel chan<- *target.Target) {
		defer close(inputChannel)
		log := logging.AddField(log, "step", "injection")
		writer := newTargetWriter(log, tr.timeouts)
		for _, target := range targets {
			if err := writer.writeTimeout(terminate, inputChannel, target, tr.timeouts.MessageTimeout); err != nil {
				log.Debugf("could not inject target %+v into first routing block: %+v", target, err)
			}
		}
	}(terminateInjectionCh, inCh)

	errCh := make(chan error)
	go func() {
		log.Infof("running pipeline")
		errCh <- pipeline.run(cancel, pause, completedTargets)
	}()

	defer close(terminateInjectionCh)
	// Receive targets from the completed channel controlled by the pipeline, while
	// waiting for termination signals or fatal errors encountered while running
	// the pipeline.
	for {
		select {
		case <-cancel:
			return <-errCh
		case <-pause:
			return <-errCh
		case err := <-errCh:
			return err
		case target := <-completedTargets:
			log.Infof("completed target: %v", target)
		}
	}
}

// NewTestRunner initializes and returns a new TestRunner object. This test
// runner will use default timeout values
func NewTestRunner() TestRunner {
	return TestRunner{
		timeouts: TestRunnerTimeouts{
			StepInjectTimeout:   config.StepInjectTimeout,
			MessageTimeout:      config.TestRunnerMsgTimeout,
			ShutdownTimeout:     config.TestRunnerShutdownTimeout,
			StepShutdownTimeout: config.TestRunnerStepShutdownTimeout,
		},
	}
}

// NewTestRunnerWithTimeouts initializes and returns a new TestRunner object with
// custom timeouts
func NewTestRunnerWithTimeouts(timeouts TestRunnerTimeouts) TestRunner {
	return TestRunner{timeouts: timeouts}
}

// State is a structure that models the current state of the test runner
type State struct {
	completedSteps   map[string]error
	completedRouting map[string]error
	completedTargets map[*target.Target]error
}

// NewState initializes a State object.
func NewState() *State {
	r := State{}
	r.completedSteps = make(map[string]error)
	r.completedRouting = make(map[string]error)
	r.completedTargets = make(map[*target.Target]error)
	return &r
}

// CompletedTargets returns a map that associates each target with its returning error.
// If the target succeeded, the error will be nil
func (r *State) CompletedTargets() map[*target.Target]error {
	return r.completedTargets
}

// CompletedRouting returns a map that associates each routing block with its returning error.
// If the routing block succeeded, the error will be nil
func (r *State) CompletedRouting() map[string]error {
	return r.completedRouting
}

// CompletedSteps returns a map that associates each step with its returning error.
// If the step succeeded, the error will be nil
func (r *State) CompletedSteps() map[string]error {
	return r.completedSteps
}

// SetRouting sets the error associated with a routing block
func (r *State) SetRouting(testStepLabel string, err error) {
	r.completedRouting[testStepLabel] = err
}

// SetTarget sets the error associated with a target
func (r *State) SetTarget(target *target.Target, err error) {
	r.completedTargets[target] = err
}

// SetStep sets the error associated with a step
func (r *State) SetStep(testStepLabel string, err error) {
	r.completedSteps[testStepLabel] = err
}

// IncompleteSteps returns a slice of step names for which the result hasn't been set yet
func (r *State) IncompleteSteps(bundles []test.TestStepBundle) []string {
	var incompleteSteps []string
	for _, bundle := range bundles {
		if _, ok := r.completedSteps[bundle.TestStepLabel]; !ok {
			incompleteSteps = append(incompleteSteps, bundle.TestStepLabel)
		}
	}
	return incompleteSteps
}
