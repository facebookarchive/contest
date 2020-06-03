// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"container/list"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
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

// router implements the routing logic that injects targets into a test step and consumes
// targets in output from the same test step
type router struct {
	log *logrus.Entry

	routingChannels routingCh
	bundle          test.TestStepBundle
	ev              testevent.EmitterFetcher

	timeouts TestRunnerTimeouts
}

// routeIn is responsible for accepting a target from the previous routing block
// and injecting it into the associated test step. Returns the numer of targets
// injected into the test step or an error upon failure
func (r *router) routeIn(terminate <-chan struct{}) (int, error) {

	stepLabel := r.bundle.TestStepLabel
	log := logging.AddField(r.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "routeIn")

	var (
		pendingTarget *target.Target
		injectionWg   sync.WaitGroup
		err           error
	)

	// terminateTargetWriter is a control channel used to signal termination to
	// the writer object which injects a target into the test step
	terminateTargetWriter := make(chan struct{})

	// `targets` is used to buffer targets coming from the previous routing blocks,
	// queueing them for injection into the TestStep. The list is accessed
	// synchronously by a single goroutine.
	targets := list.New()

	// `ingressTarget` is used to keep track of ingress times of a target into a test step
	ingressTarget := make(map[*target.Target]time.Time)

	// Channel that the injection goroutine uses to communicate back to `routeIn` the results
	// of asynchronous injection
	injectResultCh := make(chan injectionResult)

	// injectionChannels are used to inject targets into test step and return results to `routeIn`
	injectionChannels := injectionCh{stepIn: r.routingChannels.stepIn, resultCh: injectResultCh}

	log.Debugf("initializing routeIn for %s", stepLabel)
	targetWriter := newTargetWriter(log, r.timeouts)

	for {
		select {
		case <-terminate:
			err = fmt.Errorf("termination requested for routing into %s", stepLabel)
		case injectionResult := <-injectResultCh:
			log.Debugf("received injection result for %v", injectionResult.target)
			pendingTarget = nil
			if injectionResult.err != nil {
				err = fmt.Errorf("routing failed while injecting target %+v into %s", injectionResult.err, stepLabel)
				targetInErrEv := testevent.Data{EventName: target.EventTargetInErr, Target: injectionResult.target}
				if err := r.ev.Emit(targetInErrEv); err != nil {
					log.Warningf("could not emit %v event for target: %+v", targetInErrEv, *injectionResult.target)
				}
			} else {
				targetInEv := testevent.Data{EventName: target.EventTargetIn, Target: injectionResult.target}
				if err := r.ev.Emit(targetInEv); err != nil {
					log.Warningf("could not emit %v event for Target: %+v", targetInEv, *injectionResult.target)
				}
			}
		case t, chanIsOpen := <-r.routingChannels.routeIn:
			if !chanIsOpen {
				log.Debugf("routing input channel closed")
				r.routingChannels.routeIn = nil
				break
			}
			log.Debugf("received target %v in input", t)
			targets.PushFront(t)
		}

		if err != nil {
			break
		}
		if pendingTarget == nil {
			// no targets currently being injected in the test step
			if targets.Len() > 0 {
				// At least one more target available to inject to the test step
				pendingTarget = targets.Back().Value.(*target.Target)
				ingressTarget[pendingTarget] = time.Now()
				targets.Remove(targets.Back())
				injectionWg.Add(1)
				log.Debugf("writing target %v into test step", pendingTarget)
				go targetWriter.writeTargetWithResult(terminateTargetWriter, pendingTarget, injectionChannels, &injectionWg)
			} else {
				// No more targets available to inject into the teste step
				if r.routingChannels.routeIn == nil {
					log.Debugf("input channel is closed and no more targets are available, closing step input channel")
					close(r.routingChannels.stepIn)
					break
				}
			}
		}
	}
	// Signal termination to the injection routines regardless of the result of the
	// routing. If the routing completed successfully, this is a no-op. If there is an
	// injection goroutine running, wait for it to terminate, as we might have gotten
	// here after a cancellation signal.
	close(terminateTargetWriter)
	injectionWg.Wait()

	if err != nil {
		return 0, err
	}
	return len(ingressTarget), nil
}

func (r *router) emitOutEvent(t *target.Target, err error) error {

	log := logging.AddField(r.log, "step", r.bundle.TestStepLabel)
	log = logging.AddField(log, "phase", "emitOutEvent")

	if err != nil {
		targetErrPayload := target.ErrPayload{Error: err.Error()}
		payloadEncoded, err := json.Marshal(targetErrPayload)
		if err != nil {
			log.Warningf("could not encode target error ('%s'): %v", targetErrPayload, err)
		}
		rawPayload := json.RawMessage(payloadEncoded)
		targetErrEv := testevent.Data{EventName: target.EventTargetErr, Target: t, Payload: &rawPayload}
		if err := r.ev.Emit(targetErrEv); err != nil {
			return err
		}
	} else {
		targetOutEv := testevent.Data{EventName: target.EventTargetOut, Target: t}
		if err := r.ev.Emit(targetOutEv); err != nil {
			log.Warningf("could not emit %v event for target: %v", targetOutEv, *t)
		}
	}
	return nil
}

// routeOut is responsible for accepting a target from the associated test step
// and forward it to the next routing block. Returns the numer of targets
// received from the test step or an error upon failure
func (r *router) routeOut(terminate <-chan struct{}) (int, error) {

	stepLabel := r.bundle.TestStepLabel
	log := logging.AddField(r.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "routeOut")

	targetWriter := newTargetWriter(log, r.timeouts)

	var err error

	log.Debugf("initializing routeOut for %s", stepLabel)
	// `egressTarget` is used to keep track of egress times of a target from a test step
	egressTarget := make(map[*target.Target]time.Time)

	for {
		select {
		case <-terminate:
			err = fmt.Errorf("termination requested for routing into %s", r.bundle.TestStepLabel)
		case t, chanIsOpen := <-r.routingChannels.stepOut:
			if !chanIsOpen {
				log.Debugf("step output closed")
				r.routingChannels.stepOut = nil
				break
			}

			if _, targetPresent := egressTarget[t]; targetPresent {
				err = fmt.Errorf("step %s returned target %+v multiple times", r.bundle.TestStepLabel, t)
			} else {
				// Emit an event signaling that the target has left the TestStep
				if err := r.emitOutEvent(t, nil); err != nil {
					log.Warningf("could not emit out event for target: %v", *t)
				}
				// Register egress time and forward target to the next routing block
				egressTarget[t] = time.Now()
				if err := targetWriter.writeTimeout(terminate, r.routingChannels.routeOut, t, r.timeouts.MessageTimeout); err != nil {
					log.Panicf("could not forward target to the test runner: %+v", err)
				}
			}
		case targetError, chanIsOpen := <-r.routingChannels.stepErr:
			if !chanIsOpen {
				log.Debugf("step error closed")
				r.routingChannels.stepErr = nil
				break
			}

			if _, targetPresent := egressTarget[targetError.Target]; targetPresent {
				err = fmt.Errorf("step %s returned target %+v multiple times", r.bundle.TestStepLabel, targetError.Target)
			} else {
				if err := r.emitOutEvent(targetError.Target, targetError.Err); err != nil {
					log.Warningf("could not emit err event for target: %v", *targetError.Target)
				}
				egressTarget[targetError.Target] = time.Now()
				if err := targetWriter.writeTargetError(terminate, r.routingChannels.targetErr, targetError, r.timeouts.MessageTimeout); err != nil {
					log.Panicf("could not forward target (%+v) to the test runner: %v", targetError.Target, err)
				}
			}
		}
		if err != nil {
			break
		}
		if r.routingChannels.stepErr == nil && r.routingChannels.stepOut == nil {
			log.Debugf("output and error channel from step are closed, routeOut should terminate")
			close(r.routingChannels.routeOut)
			break
		}
	}

	if err != nil {
		return 0, err
	}
	return len(egressTarget), nil

}

// route implements the routing logic from the previous routing block to the test step
// and from the test step to the next routing block
func (r *router) route(terminate <-chan struct{}, resultCh chan<- routeResult) {

	var (
		inTargets, outTargets int
		inErr, outErr, err    error
		routeWg               sync.WaitGroup
	)

	routeWg.Add(1)
	go func() {
		defer routeWg.Done()
		inTargets, inErr = r.routeIn(terminate)
	}()

	routeWg.Add(1)
	go func() {
		defer routeWg.Done()
		outTargets, outErr = r.routeOut(terminate)
	}()

	routeWg.Wait()
	if inErr != nil {
		err = inErr
	}
	if outErr != nil {
		err = outErr
	}

	if err == nil && inTargets != outTargets {
		err = fmt.Errorf("step %s completed but did not return all injected Targets (%d!=%d)", r.bundle.TestStepLabel, inTargets, outTargets)
	}

	// Send the result to the test runner, which is expected to be listening
	// within `MessageTimeout`. If that's not the case, we hit an unrecovrable
	// condition.
	select {
	case resultCh <- routeResult{bundle: r.bundle, err: err}:
	case <-time.After(r.timeouts.MessageTimeout):
		log.Panicf("could not send routing block result")
	}
}

func newRouter(log *logrus.Entry, bundle test.TestStepBundle, routingChannels routingCh, ev testevent.EmitterFetcher, timeouts TestRunnerTimeouts) *router {
	r := router{log: log, bundle: bundle, routingChannels: routingChannels, ev: ev, timeouts: timeouts}
	return &r
}

// pipeline represents a sequence of steps through which targets flow. A pipeline could implement
// either a test sequence or a cleanup sequence
type pipeline struct {
	log *logrus.Entry

	bundles []test.TestStepBundle

	jobID types.JobID
	runID types.RunID

	state *State
	test  *test.Test

	timeouts TestRunnerTimeouts

	// ctrlChannels represents a set of result and completion channels for this pipeline,
	// used to collect the results of routing blocks, steps and targets completing
	// the pipeline. It's available only after having initialized the pipeline
	ctrlChannels *pipelineCtrlCh

	// numIngress represents the total number of targets that have been seen entering
	// the pipeline. This number is set by the first routing block as soon as the injection
	// terminates
	numIngress uint64
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
func (p *pipeline) waitTargets(terminate <-chan struct{}, completedCh chan<- *target.Target) error {

	log := logging.AddField(p.log, "phase", "waitTargets")

	var (
		err                  error
		completedTarget      *target.Target
		completedTargetError error
	)

	outChannel := p.ctrlChannels.targetOut

	writer := newTargetWriter(log, p.timeouts)
	for {
		select {
		case target, ok := <-outChannel:
			if !ok {
				log.Debugf("pipeline output channel was closed, no more targets will come through")
				outChannel = nil
				break
			}
			completedTarget = target
		case <-terminate:
			// When termination is signaled just stop wait. It is up
			// to the caller to decide how to further handle pipeline termination.
			log.Debugf("termination requested")
			return nil
		case res := <-p.ctrlChannels.routingResultCh:
			err = res.err
			p.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-p.ctrlChannels.stepResultCh:
			err = res.err
			if err != nil {
				payload, jmErr := json.Marshal(err.Error())
				if jmErr != nil {
					log.Warningf("failed to marshal error string to JSON: %v", jmErr)
					continue
				}
				rm := json.RawMessage(payload)
				header := testevent.Header{
					JobID:         res.jobID,
					RunID:         res.runID,
					TestName:      p.test.Name,
					TestStepLabel: res.bundle.TestStepLabel,
				}
				ev := storage.NewTestEventEmitterFetcher(header)
				// this event is not associated to any target, e.g. a plugin has returned an error.
				errEv := testevent.Data{EventName: EventTestError, Target: nil, Payload: &rm}
				// emit test event containing the completion error
				if err := ev.Emit(errEv); err != nil {
					log.Warningf("could not emit completion error event %v", errEv)
				}
			}
			p.state.SetStep(res.bundle.TestStepLabel, res.err)

		case targetErr := <-p.ctrlChannels.targetErr:
			completedTarget = targetErr.Target
			completedTargetError = targetErr.Err
		}

		if err != nil {
			return err
		}

		if outChannel == nil {
			log.Debugf("no more targets to wait, output channel is closed")
			break
		}

		if completedTarget != nil {
			p.state.SetTarget(completedTarget, completedTargetError)
			log.Debugf("writing target %+v on the completed channel", completedTarget)
			if err := writer.writeTimeout(terminate, completedCh, completedTarget, p.timeouts.MessageTimeout); err != nil {
				log.Panicf("could not write completed target: %v", err)
			}
			completedTarget = nil
			completedTargetError = nil
		}

		numIngress := atomic.LoadUint64(&p.numIngress)
		numCompleted := uint64(len(p.state.CompletedTargets()))
		log.Debugf("targets completed: %d, expected: %d", numCompleted, numIngress)
		if numIngress != 0 && numCompleted == numIngress {
			log.Debugf("no more targets to wait, all targets (%d) completed", numCompleted)
			break
		}
	}
	// The test run completed, we have collected all Targets. TestSteps might have already
	// closed `ch.out`, in which case the pipeline terminated correctly (channels are closed
	// in a "domino" sequence, so seeing the last channel closed indicates that the
	// sequence of close operations has completed). If `ch.out` is still open,
	// there are still TestSteps that might have not returned. Wait for all
	// TestSteps to complete or `StepShutdownTimeout` to occur.
	log.Infof("waiting for all steps to complete")
	return p.waitSteps()
}

// waitTermination reads results coming from result channels waiting
// for the pipeline to completely shutdown before `ShutdownTimeout` occurs. A
// "complete shutdown" means that all TestSteps and routing blocks have sent
// downstream their results.
func (p *pipeline) waitTermination() error {

	log := logging.AddField(p.log, "phase", "waitTermination")
	log.Printf("waiting for pipeline to terminate")

	for {

		log.Debugf("steps completed: %d", len(p.state.CompletedSteps()))
		log.Debugf("routing completed: %d", len(p.state.CompletedRouting()))
		stepsCompleted := len(p.state.CompletedSteps()) == len(p.bundles)
		routingCompleted := len(p.state.CompletedRouting()) == len(p.bundles)
		if stepsCompleted && routingCompleted {
			return nil
		}

		select {
		case <-time.After(p.timeouts.ShutdownTimeout):
			incompleteSteps := p.state.IncompleteSteps(p.bundles)
			if len(incompleteSteps) > 0 {
				return &cerrors.ErrTestStepsNeverReturned{StepNames: incompleteSteps}
			}
			return fmt.Errorf("pipeline did not return but all test steps completed")
		case res := <-p.ctrlChannels.routingResultCh:
			p.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-p.ctrlChannels.stepResultCh:
			p.state.SetStep(res.bundle.TestStepLabel, res.err)
		}
	}
}

// waitSteps reads results coming from result channels until `StepShutdownTimeout`
// occurs or an error is encountered. It then checks whether TestSteps and routing
// blocks have all returned correctly. If not, it returns an error.
func (p *pipeline) waitSteps() error {

	log := logging.AddField(p.log, "phase", "waitSteps")

	var err error

	log.Debugf("waiting fot test steps to terminate")
	for {
		select {
		case <-time.After(p.timeouts.StepShutdownTimeout):
			log.Warningf("timed out waiting for steps to complete after %v", p.timeouts.StepShutdownTimeout)
			incompleteSteps := p.state.IncompleteSteps(p.bundles)
			if len(incompleteSteps) > 0 {
				err = &cerrors.ErrTestStepsNeverReturned{StepNames: incompleteSteps}
				break
			}
			if len(p.state.CompletedRouting()) != len(p.bundles) {
				err = fmt.Errorf("not all routing completed: %d!=%d", len(p.state.CompletedRouting()), len(p.bundles))
			}
		case res := <-p.ctrlChannels.routingResultCh:
			log.Debugf("received routing block result for %s", res.bundle.TestStepLabel)
			p.state.SetRouting(res.bundle.TestStepLabel, res.err)
			err = res.err
		case res := <-p.ctrlChannels.stepResultCh:
			log.Debugf("received step result for %s", res.bundle.TestStepLabel)
			p.state.SetStep(res.bundle.TestStepLabel, res.err)
			err = res.err
		}
		if err != nil {
			return err
		}
		log.Debugf("steps completed: %d, expected: %d", len(p.state.CompletedSteps()), len(p.bundles))
		log.Debugf("routing completed: %d, expected: %d", len(p.state.CompletedRouting()), len(p.bundles))
		stepsCompleted := len(p.state.CompletedSteps()) == len(p.bundles)
		routingCompleted := len(p.state.CompletedRouting()) == len(p.bundles)
		if stepsCompleted && routingCompleted {
			break
		}
	}

	return nil
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
	for position, testStepBundle := range p.bundles {

		// Input and output channels for the TestStep
		stepInCh := make(chan *target.Target)
		stepOutCh := make(chan *target.Target)
		stepErrCh := make(chan cerrors.TargetError)

		routeOut = make(chan *target.Target)

		// First step of the pipeline, keep track of the routeIn channel as this is
		// going to be used to injects targets into the pipeline from outside. Also
		// add an intermediate goroutine which keeps track of how many targets have
		// been injected into the pipeline
		if position == 0 {
			routeInFirst = make(chan *target.Target)
			routeInStep := routeIn
			go func() {
				defer close(routeInStep)
				numIngress := uint64(0)
				for t := range routeInFirst {
					routeInStep <- t
					numIngress++

				}
				atomic.StoreUint64(&p.numIngress, numIngress)
			}()
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
			TestName:      p.test.Name,
			TestStepLabel: testStepBundle.TestStepLabel,
		}
		ev := storage.NewTestEventEmitterFetcher(Header)

		router := newRouter(p.log, testStepBundle, routingChannels, ev, p.timeouts)
		go router.route(routingCancelCh, routingResultCh)
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
func (p *pipeline) run(cancel, pause <-chan struct{}, completedTargetsCh chan<- *target.Target) error {

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
		errCh <- p.waitTargets(cancelWaitTargetsCh, completedTargetsCh)
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

func newPipeline(log *logrus.Entry, bundles []test.TestStepBundle, test *test.Test, jobID types.JobID, runID types.RunID, timeouts TestRunnerTimeouts) *pipeline {
	p := pipeline{log: log, bundles: bundles, jobID: jobID, runID: runID, test: test, timeouts: timeouts}
	p.state = NewState()
	return &p
}

// Run implements the main logic of the TestRunner, i.e. the instantiation and
// connection of the TestSteps, routing blocks and pipeline runner.
func (tr *TestRunner) Run(cancel, pause <-chan struct{}, test *test.Test, targets []*target.Target, jobID types.JobID, runID types.RunID) error {

	if len(test.TestStepsBundles) == 0 {
		return fmt.Errorf("no steps to run for test")
	}

	// rootLog is propagated to all the subsystems of the pipeline
	rootLog := logging.GetLogger("pkg/runner")
	fields := make(map[string]interface{})
	fields["jobid"] = jobID
	fields["runid"] = runID
	rootLog = logging.AddFields(rootLog, fields)

	log := logging.AddField(rootLog, "phase", "run")
	testPipeline := newPipeline(logging.AddField(rootLog, "entity", "test_pipeline"), test.TestStepsBundles, test, jobID, runID, tr.timeouts)

	log.Infof("setting up pipeline")
	completedTargets := make(chan *target.Target)
	inCh := testPipeline.init(cancel, pause)

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
		errCh <- testPipeline.run(cancel, pause, completedTargets)
	}()

	defer close(terminateInjectionCh)
	// Receive targets from the completed channel controlled by the pipeline, while
	// waiting for termination signals or fatal errors encountered while running
	// the pipeline.
	for {
		select {
		case <-cancel:
			err := <-errCh
			log.Debugf("test runner terminated, returning %v", err)
			return err
		case <-pause:
			err := <-errCh
			log.Debugf("test runner terminated, returning %v", err)
			return err
		case err := <-errCh:
			log.Debugf("test runner terminated, returning %v", err)
			return err
		case target := <-completedTargets:
			log.Infof("test runner completed target: %v", target)
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
