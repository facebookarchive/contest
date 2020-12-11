// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/sirupsen/logrus"
)

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
func (p *pipeline) runStep(ctx statectx.Context, jobID types.JobID, runID types.RunID, bundle test.TestStepBundle, stepCh stepCh, resultCh chan<- stepResult, ev testevent.EmitterFetcher) {

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
	stepIn, onFirstTargetChan, onNoTargetsChan := waitForFirstTarget(ctx.PausedOrDoneCtx(), stepCh.stepIn)

	haveTargets := false
	select {
	case <-onFirstTargetChan:
		log.Debugf("first target received, will start step")
		haveTargets = true
	case <-ctx.Done():
		log.Debugf("cancelled")
	case <-ctx.Paused():
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
		err = bundle.TestStep.Run(ctx, channels, bundle.Parameters, ev)
	}

	log.Debugf("step %s returned", bundle.TestStepLabel)

	// Check if we are shutting down. If so, do not perform sanity checks on the
	// channels but return immediately as the TestStep itself probably returned
	// because it honored the termination signal.

	cancellationAsserted := ctx.PausedOrDoneCtx().Err() == statectx.ErrCanceled
	pauseAsserted := ctx.PausedOrDoneCtx().Err() == statectx.ErrPaused

	if cancellationAsserted && err == nil {
		err = fmt.Errorf("test step cancelled")
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
		err = fmt.Errorf("step %s returned, but input channel is not closed (api violation; case 0)", stepLabel)

	default:
		// stepCh.stepIn is not closed, and a read operation would block. The TestStep
		// does not comply with the API (see above).
		err = fmt.Errorf("step %s returned, but input channel is not closed (api violation; case 1)", stepLabel)
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
func (p *pipeline) waitTargets(ctx context.Context, completedCh chan<- *target.Target) error {

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
		case t, ok := <-outChannel:
			if !ok {
				log.Debugf("pipeline output channel was closed, no more targets will come through")
				outChannel = nil
				break
			}
			completedTarget = t
		case <-ctx.Done():
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
			if err := writer.writeTimeout(ctx, completedCh, completedTarget, p.timeouts.MessageTimeout); err != nil {
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

	if len(p.bundles) == 0 {
		return fmt.Errorf("no bundles specified for waitTermination")
	}

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

	if len(p.bundles) == 0 {
		return fmt.Errorf("no bundles specified for waitSteps")
	}

	log := logging.AddField(p.log, "phase", "waitSteps")

	var err error

	log.Debugf("waiting for test steps to terminate")
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
func (p *pipeline) init() (routeInFirst chan *target.Target) {
	p.log.Debugf("starting")

	if p.ctrlChannels != nil {
		p.log.Panicf("pipeline is already initialized, control channel are already configured")
	}

	var (
		routeOut chan *target.Target
		routeIn  chan *target.Target
	)

	ctx, pause, cancel := statectx.NewContext()

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
		ev := storage.NewTestEventEmitterFetcherWithAllowedEvents(Header, &testStepBundle.AllowedEvents)

		router := newStepRouter(p.log, testStepBundle, routingChannels, ev, p.timeouts)
		go router.route(ctx.PausedOrDoneCtx(), routingResultCh)
		go p.runStep(ctx, p.jobID, p.runID, testStepBundle, stepChannels, stepResultCh, ev)
		// The input of the next routing block is the output of the current routing block
		routeIn = routeOut
	}

	p.ctrlChannels = &pipelineCtrlCh{
		routingResultCh: routingResultCh,
		stepResultCh:    stepResultCh,
		targetErr:       targetErrCh,
		targetOut:       routeOut,

		stateCtx: ctx,
		cancel:   cancel,
		pause:    pause,
	}

	return
}

// run is a blocking method which executes the pipeline until successful or failed termination
func (p *pipeline) run(ctx statectx.Context, completedTargetsCh chan<- *target.Target) error {
	p.log.Debugf("run")
	if p.ctrlChannels == nil {
		p.log.Panicf("pipeline is not initialized, control channels are not available")
	}

	// Wait for the pipeline to complete. If an error occurrs, cancel all TestSteps
	// and routing blocks and wait again for completion until shutdown timeout occurrs.
	p.log.Infof("waiting for pipeline to complete")

	completionError := p.waitTargets(ctx.PausedOrDoneCtx(), completedTargetsCh)
	pauseAsserted := ctx.PausedOrDoneCtx().Err() == statectx.ErrPaused
	cancellationAsserted := ctx.PausedOrDoneCtx().Err() == statectx.ErrCanceled

	if completionError != nil || cancellationAsserted {
		// If the Test has encountered an error or cancellation has been asserted,
		// terminate routing and and propagate the cancel signal to the steps
		if cancellationAsserted {
			p.log.Infof("cancellation was asserted")
		} else {
			p.log.Warningf("test failed to complete: %v. Forcing cancellation.", completionError)
		}

		p.ctrlChannels.cancel()
	}

	if pauseAsserted {
		// If pause signal has been asserted, terminate routing and propagate the pause signal to the steps.
		p.log.Warningf("received pause request")
		p.ctrlChannels.pause()
	}

	// If either cancellation or pause have been asserted, we need to wait for the
	// pipeline to terminate
	if cancellationAsserted || pauseAsserted || completionError != nil {
		signal := "cancellation"
		if pauseAsserted {
			signal = "pause"
		}
		terminationError := p.waitTermination()
		if terminationError != nil {
			p.log.Infof("test did not terminate correctly after %s signal: %v", signal, terminationError)
		} else {
			p.log.Infof("test terminated correctly after %s signal", signal)
		}
		if completionError != nil {
			return completionError
		}
		return terminationError
	}
	p.log.Infof("completed")
	return nil
}

func newPipeline(log *logrus.Entry, bundles []test.TestStepBundle, test *test.Test, jobID types.JobID, runID types.RunID, timeouts TestRunnerTimeouts) *pipeline {
	p := pipeline{log: log, bundles: bundles, jobID: jobID, runID: runID, test: test, timeouts: timeouts}
	p.state = NewState()
	return &p
}
