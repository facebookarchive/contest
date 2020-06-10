// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/sirupsen/logrus"
)

// pipelineCtrlCh represents multiple result and control channels that the pipeline uses
// to collect results from routing blocks, steps and target completing the test and to
//  signa cancellation to various pipeline subsystems
type pipelineCtrlCh struct {
	routingResultCh  <-chan routeResult
	stepResultCh     <-chan stepResult
	targetResultCh   <-chan *target.Result
	targetExpectedCh <-chan uint64

	// cancelInternal is a signal which triggers cancellation in the internal control logic and steps
	cancelInternalCh chan struct{}
	// pauseInternal is a control channel used to pause the steps of the pipeline
	pauseInternalCh chan struct{}
}

// pipeline represents a sequence of steps through which targets flow. A pipeline could implement
// either a test sequence or a cleanup sequence
type pipeline struct {
	log *logrus.Entry

	bundles []test.StepBundle

	jobID types.JobID
	runID types.RunID

	state *State
	test  *test.Test

	timeouts TestRunnerTimeouts

	// ctrlChannels represents a set of result and completion channels for this pipeline,
	// used to collect the results of routing blocks, steps and targets completing
	// the pipeline. It's available only after having initialized the pipeline
	ctrlChannels *pipelineCtrlCh
}

// runStep runs synchronously a Step and peforms sanity checks on the status
// of the input/output channels on the defer control path. When the Step returns,
// the associated output channels are closed. This signals to the routing subsytem
// that this step of the pipeline will not be producing any more targets. The API
// dictates that the Step can only return if its input channel has been closed,
// which indicates that no more targets will be submitted to that step. If the
// Step does not comply with this rule, an error is sent downstream to the pipeline
// runner, which will in turn shutdown the whole pipeline and flag the Step
// as misbehaving. All attempts to send downstream a result after a Step complete
// are protected by a timeout and fail open. This because it's not guaranteed that
// the TestRunner will still be listening on the result channels. If the Step hangs
// indefinitely and does not respond to cancellation signals, the TestRunner will
// flag it as misbehaving and return. If the Step returns once the TestRunner
// has completed, it will timeout trying to write on the result channel.
func (p *pipeline) runStep(cancel, pause <-chan struct{}, jobID types.JobID, runID types.RunID, bundle test.StepBundle, stepCh stepCh, resultCh chan<- stepResult, ev testevent.EmitterFetcher) {

	stepLabel := bundle.StepLabel
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
	// then we call `bundle.Step.Run()`.
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
		// Run the Step and defer to the return path the handling of panic
		// conditions. If multiple error conditions occur, send downstream only
		// the first error encountered.
		channels := test.StepChannels{In: stepIn, Out: stepCh.stepOut}
		err = bundle.Step.Run(cancel, pause, channels, bundle.Parameters, ev)
	}

	log.Debugf("step %s returned", bundle.StepLabel)
	var (
		cancellationAsserted bool
		pauseAsserted        bool
	)
	// Check if we are shutting down. If so, do not perform sanity checks on the
	// channels but return immediately as the Step itself probably returned
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
			log.Warningf("sending error back from Step runner timed out after %v: %v", timeout, err)
		}
		return
	}

	// If the Step has crashed with an error, return immediately the result to the TestRunner
	// which in turn will issue a cancellation signal to the pipeline
	if err != nil {
		select {
		case resultCh <- stepResult{jobID: jobID, runID: runID, bundle: bundle, err: err}:
		case <-time.After(timeout):
			log.Warningf("sending error back from test step runner (%s) timed out after %v: %v", stepLabel, timeout, err)
		}
		return
	}

	// Perform sanity checks on the status of the channels. The Step API
	// mandates that output and error channels shall not be closed by the
	// Step itself. If the Step does not comply with the API, it is
	// flagged as misbehaving.
	select {
	case _, ok := <-stepCh.stepIn:
		if !ok {
			break
		}
		// stepCh.stepIn is not closed, but the Step returned, which is a violation
		// of the API. Record the error if no other error condition has been seen.
		if err == nil {
			err = fmt.Errorf("step %s returned, but input channel is not closed (api violation; case 0)", stepLabel)
		}
	default:
		// stepCh.stepIn is not closed, and a read operation would block. The Step
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
				err = &cerrors.ErrStepClosedChannels{StepName: stepLabel}
			}
		}
	default:
		// stepCh.stepOut is open. Flag it for closure to signal to the routing subsystem that
		// no more Targets will go through from this channel.
		close(stepCh.stepOut)
	}

	select {
	case resultCh <- stepResult{jobID: jobID, runID: runID, bundle: bundle, err: err}:
	case <-time.After(timeout):
		log.Warningf("sending error back from step runner (%s) timed out after %v: %v", stepLabel, timeout, err)
		return
	}
}

// emitStepEvent emits a failure event if a step fail
func (p *pipeline) emitStepEvent(result *stepResult) error {
	if result.err == nil {
		return nil
	}

	payload, jmErr := json.Marshal(result.err.Error())
	if jmErr != nil {
		return fmt.Errorf("failed to marshal error string to JSON: %v", jmErr)
	}
	rm := json.RawMessage(payload)
	ev := storage.NewTestEventEmitterFetcher(testevent.Header{
		JobID:     result.jobID,
		RunID:     result.runID,
		TestName:  p.test.Name,
		StepLabel: result.bundle.StepLabel,
	})
	errEv := testevent.Data{
		EventName: EventTestError,
		// this event is not associated to any target, e.g. a plugin has
		// returned an error.
		Target:  nil,
		Payload: &rm,
	}
	// emit test event containing the completion error
	return ev.Emit(errEv)
}

// waitControl reads results coming from result channels (for steps and routing blocks)
// until a timeout occurrs. The error handling is different depending on whether
// a timeout has been set from outside: if not timeout it set, we return at the first error
// encountered.
func (p *pipeline) waitControl(terminate <-chan struct{}, routingResultCh <-chan routeResult, stepResultCh <-chan stepResult, returnOnErr bool) error {

	log := logging.AddField(p.log, "phase", "waitControl")

	var err error

	log.Debugf("waiting control logic")
	for {

		numSteps := len(p.state.CompletedSteps())
		numRouting := len(p.state.CompletedRouting())
		expected := len(p.bundles)
		log.Debugf("steps %d, routing %d, expected: %d", numSteps, numRouting, expected)
		if numSteps == expected && numRouting == expected {
			return nil
		}

		select {
		case <-terminate:
			return &ErrTerminationRequested{}
		case routingResult := <-routingResultCh:
			log.Debugf("received routing block result for %s", routingResult.bundle.StepLabel)
			p.state.SetRouting(routingResult.bundle.StepLabel, routingResult.err)
			err = routingResult.err
		case stepResult := <-stepResultCh:
			log.Debugf("received step result for %s", stepResult.bundle.StepLabel)
			p.state.SetStep(stepResult.bundle.StepLabel, stepResult.err)
			err = stepResult.err
			if err != nil {
				eventErr := p.emitStepEvent(&stepResult)
				if eventErr != nil {
					log.Warningf("failed to emit step vent error: %v", eventErr)
					continue
				}
			}
		}
		if err != nil && returnOnErr {
			log.Debugf("step or channel returned fatal error %v", err)
			return err
		}
	}
}

// wait reads results coming from results channels until all have completed or
// an error occurs. If all Targets complete successfully, it checks whether
// Steps and routing blocks have completed as well. If not, returns an error.
// Termination is signalled via terminate channel.
func (p *pipeline) waitTargets(terminate <-chan struct{}, targetResultCh <-chan *target.Result, targetExpectedCh <-chan uint64, outTargetCh chan<- *target.Target) error {

	log := logging.AddField(p.log, "phase", "waitTargets")
	var numTargets uint64

	writer := newTargetWriter(log, p.timeouts)

	for {
		select {
		case <-terminate:
			return &ErrTerminationRequested{}
		case numTargets = <-targetExpectedCh:
			targetExpectedCh = nil
		case targetResult, isChanOpen := <-targetResultCh:
			if !isChanOpen {
				log.Debugf("pipeline output channel was closed, no more targets will come through")
				targetResultCh = nil
			} else {
				timeout := p.timeouts.MessageTimeout
				p.state.SetTarget(targetResult.Target, targetResult.Err)
				log.Debugf("completed taget %v", targetResult.Target)
				if err := writer.writeTarget(terminate, outTargetCh, targetResult.Target, timeout); err != nil {
					log.Panicf("could not write completed target: %v", err)
				}
			}
		}
		// if we have been told how many targets to expect, compare against how many we got
		completed := uint64(len(p.state.CompletedTargets()))
		expectedTargets := "unkonwn"
		if numTargets != 0 {
			expectedTargets = fmt.Sprintf("%d", numTargets)
		}

		log.Infof("targets completed %d, expected %s", completed, expectedTargets)
		if targetExpectedCh == nil {
			if numTargets == completed {
				return nil
			}
			if targetResultCh == nil && numTargets != completed {
				return fmt.Errorf("not all targets completed, but target channel is closed")
			}
		}
	}
}

func (p *pipeline) setupRouteIn(routeOut, routeIn chan *target.Target, targetExpectedCh chan<- uint64, targetResultCh chan *target.Result) chan *target.Target {
	// first step of the pipeline, keep track of the routeIn channel as this is
	// going to be used to injects targets into the pipeline from outside. Also
	// add an intermediate goroutine which keeps track of how many targets have
	// been injected into the pipeline
	routeInFirst := make(chan *target.Target)
	routeInStep := routeIn
	go func() {
		defer close(routeInStep)
		defer close(targetExpectedCh)
		var completed uint64
		for t := range routeInFirst {
			p.log.Debugf("target in input to first routing block: %v", t)
			routeInStep <- t
			completed++
		}
		targetExpectedCh <- completed
	}()
	return routeInFirst
}

func (p *pipeline) setupRoutOut(routeOut, routeIn chan *target.Target, targetResultCh chan *target.Result) {
	// test runner will be waiting for results on targetResultCh, so we need to
	// forward all targets in output from the last routing block to the targetResultCh
	go func() {
		defer close(targetResultCh)
		for t := range routeOut {
			p.log.Debugf("target in output from the last routing block: %v", t)
			targetResultCh <- &target.Result{Target: t}
		}
	}()
}

// init initializes the pipeline by connecting steps and routing blocks. The result of pipeline
// initialization is a set of control/result channels assigned to the pipeline object. The pipeline
// input channel is returned.
func (p *pipeline) init(cancel, pause <-chan struct{}) (routeInFirst chan *target.Target) {
	p.log.Debugf("starting")

	if p.ctrlChannels != nil {
		p.log.Panicf("pipeline is already initialized, control channel are already configured")
	}

	var routeOut, routeIn chan *target.Target

	targetExpectedCh := make(chan uint64)
	cancelInternalCh := make(chan struct{})
	pauseInternalCh := make(chan struct{})

	// result channels used to communicate result information from the routing blocks
	// and step executors
	routingResultCh := make(chan routeResult)
	stepResultCh := make(chan stepResult)
	targetResultCh := make(chan *target.Result)

	routeIn = make(chan *target.Target)
	for position, testStepBundle := range p.bundles {

		// Input and output channels for the Step
		stepInCh := make(chan *target.Target)
		stepOutCh := make(chan *target.Result)
		routeOut = make(chan *target.Target)

		if position == 0 {
			routeInFirst = p.setupRouteIn(routeOut, routeIn, targetExpectedCh, targetResultCh)
		}
		if position == len(p.bundles)-1 {
			p.setupRoutOut(routeOut, routeIn, targetResultCh)
		}

		stepChannels := stepCh{stepIn: stepInCh, stepOut: stepOutCh}
		routingChannels := routingCh{routeIn: routeIn, routeOut: routeOut, stepIn: stepInCh, stepOut: stepOutCh, targetResult: targetResultCh}

		// Build the Header that the the Step will be using for emitting events
		Header := testevent.Header{JobID: p.jobID, RunID: p.runID, TestName: p.test.Name, StepLabel: testStepBundle.StepLabel}
		ev := storage.NewTestEventEmitterFetcher(Header)

		router := newRouter(p.log, testStepBundle, routingChannels, ev, p.timeouts)
		go router.route(cancelInternalCh, pauseInternalCh, routingResultCh)
		go p.runStep(cancelInternalCh, pauseInternalCh, p.jobID, p.runID, testStepBundle, stepChannels, stepResultCh, ev)
		// The input of the next routing block is the output of the current routing block
		routeIn = routeOut
	}

	p.ctrlChannels = &pipelineCtrlCh{
		routingResultCh:  routingResultCh,
		stepResultCh:     stepResultCh,
		targetResultCh:   targetResultCh,
		targetExpectedCh: targetExpectedCh,
		cancelInternalCh: cancelInternalCh,
		pauseInternalCh:  pauseInternalCh,
	}
	return
}

// run is a blocking method which executes the pipeline until successful or failed termination
func (p *pipeline) run(cancel, pause <-chan struct{}, outTargetsCh chan<- *target.Target) error {

	log := logging.AddField(p.log, "phase", "pipeline_run")

	log.Debugf("run")
	if p.ctrlChannels == nil {
		p.log.Panicf("pipeline is not initialized, control channels are not available")
	}

	log.Infof("running pipeline")
	var (
		waitControlError, waitTargetsError  error
		cancellationAsserted, pauseAsserted bool
	)

	errWaitControlCh := make(chan error)
	errWaitTargetsCh := make(chan error)
	cancelWaitsCh := make(chan struct{})

	go func() {
		errWaitTargetsCh <- p.waitTargets(cancelWaitsCh, p.ctrlChannels.targetResultCh, p.ctrlChannels.targetExpectedCh, outTargetsCh)
	}()
	go func() {
		errWaitControlCh <- p.waitControl(cancelWaitsCh, p.ctrlChannels.routingResultCh, p.ctrlChannels.stepResultCh, true)
	}()

	for {
		select {
		case waitTargetsError = <-errWaitTargetsCh:
			errWaitTargetsCh = nil
		case waitControlError = <-errWaitControlCh:
			errWaitControlCh = nil
			log.Debugf("control returned: %v", waitControlError)
			if waitControlError != nil {
				cancellationAsserted = true
				close(p.ctrlChannels.cancelInternalCh)
			}
		case <-cancel:
			log.Debugf("cancellation asserted, propagating internal cancellation signal")
			close(p.ctrlChannels.cancelInternalCh)
			cancellationAsserted = true
		case <-pause:
			log.Debugf("pause asserted, propagating internal pause signal")
			close(p.ctrlChannels.pauseInternalCh)
			pauseAsserted = true
		}
		if cancellationAsserted || pauseAsserted {
			break
		}
		// if we finished waiting for targets we should return regardless of the outcome
		// Control will be waited for further below, if necessary
		if errWaitTargetsCh == nil {
			break
		}
	}

	waitTermination := func(ch chan error, name string) {
		if ch != nil {
			log.Debugf("waiting for %s termination", name)
			<-ch
			log.Debugf("%s terminated", name)
		}
	}
	close(cancelWaitsCh)
	waitTermination(errWaitControlCh, "control")
	waitTermination(errWaitTargetsCh, "targets")

	var (
		terminationError   error
		shutdownControlErr error
	)

	// control did not complete, or completed with a failure?
	if errWaitControlCh != nil || waitControlError != nil {
		waitControlShutdown := make(chan error)
		log.Warningf("control di not complete, controlError %v, pause: %v, cancel: %v", waitControlError, cancellationAsserted, pauseAsserted)
		cancelControlCh := make(chan struct{})
		go func() {
			waitControlShutdown <- p.waitControl(cancelControlCh, p.ctrlChannels.routingResultCh, p.ctrlChannels.stepResultCh, false)
		}()
		select {
		case <-time.After(p.timeouts.ShutdownTimeout):
			close(cancelControlCh)
			<-waitControlShutdown
			log.Debugf("state: %+v", p.state)
			shutdownControlErr = fmt.Errorf("control did not shutdown within %v", p.timeouts.ShutdownTimeout)
		case err := <-waitControlShutdown:
			if err != nil {
				shutdownControlErr = fmt.Errorf("control failed shutdown: %v", err)
			}
		}
		if waitControlError != nil {
			terminationError = fmt.Errorf("control error: %w", waitControlError)
		}
		if shutdownControlErr != nil {
			terminationError = fmt.Errorf("control error: %v, shutdown error: %v", waitControlError, shutdownControlErr)
		}
		log.Debugf("pipeline completed (err: %v)", terminationError)
		return terminationError
	}

	if cancellationAsserted || pauseAsserted {
		return fmt.Errorf("cancellation or pause asserted")
	}
	log.Debugf("pipeline completed (err: %v)", waitTargetsError)
	return waitTargetsError
}

func newPipeline(log *logrus.Entry, bundles []test.StepBundle, test *test.Test, jobID types.JobID, runID types.RunID, timeouts TestRunnerTimeouts) *pipeline {
	p := pipeline{log: log, bundles: bundles, jobID: jobID, runID: runID, test: test, timeouts: timeouts}
	p.state = NewState()
	return &p
}
