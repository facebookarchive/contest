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
	routingResultCh chan routeResult
	stepResultCh    chan stepResult
	targetErrCh     chan *target.Result

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

func closeAfter(ch chan struct{}, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		close(ch)
	}
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

	// pipeline guarantees to collect all step results
	resultCh <- stepResult{jobID: jobID, runID: runID, bundle: bundle, err: err}
	log.Infof("sent result back from step runner (%s): %v", stepLabel, err)
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

// waitStepsAndRouting reads results coming from result channels for steps and routing blocks
// until a timeout occurrs.
func (p *pipeline) waitStepsAndRouting(terminate <-chan struct{}, routingResultCh <-chan routeResult, stepResultCh <-chan stepResult, loopForever bool) error {

	log := logging.AddField(p.log, "phase", "waitControl")

	var err error
	log.Debugf("waiting control logic")
	expected := len(p.bundles)
	stepsTimeout := make(chan struct{})

	var engageTimeout sync.Once

	for {
		completedSteps := len(p.state.CompletedSteps()) == expected
		completedRouting := len(p.state.CompletedRouting()) == expected
		log.Debugf("steps %d, routing %d, expected: %d", len(p.state.CompletedSteps()), len(p.state.CompletedRouting()), expected)
		if completedRouting && completedSteps {
			// both routing and steps completed
			break
		}
		if completedRouting && !completedSteps && !loopForever {
			log.Debugf("engaging close after")
			// routing completed, steps did not. We expect steps to return within a timeout.
			// If we have been asked to loop forver, we'll just wait to be terminated
			engageTimeout.Do(func() { go func() { closeAfter(stepsTimeout, p.timeouts.ShutdownTimeout) }() })
		}

		select {
		case <-terminate:
			return &ErrTerminationRequested{}
		case <-stepsTimeout:
			log.Debugf("steps timed out")
			err = fmt.Errorf("steps did not completed within timeout")
		case routingResult := <-routingResultCh:
			log.Debugf("received routing block result for %s", routingResult.bundle.StepLabel)
			p.state.SetRouting(routingResult.bundle.StepLabel, routingResult.err)
			err = routingResult.err
		case stepResult := <-stepResultCh:
			log.Debugf("received step result for %s", stepResult.bundle.StepLabel)
			p.state.SetStep(stepResult.bundle.StepLabel, stepResult.err)
			err = stepResult.err
			if err != nil {
				if eventErr := p.emitStepEvent(&stepResult); eventErr != nil {
					log.Warningf("failed to emit step vent error: %v", eventErr)
				}
			}
		}
		if err != nil && !loopForever {
			log.Debugf("step or channel returned fatal error %v", err)
			return err
		}
	}

	return nil
}

// init initializes the pipeline by connecting steps and routing blocks. The result of pipeline
// initialization is a set of control/result channels assigned to the pipeline object. The pipeline
// input channel is returned.
func (p *pipeline) init(cancel, pause <-chan struct{}) (in chan *target.Target, out chan *target.Target) {
	p.log.Debugf("starting")

	if p.ctrlChannels != nil {
		p.log.Panicf("pipeline is already initialized, control channel are already configured")
	}

	var routeOut, routeIn chan *target.Target

	cancelInternalCh := make(chan struct{})
	pauseInternalCh := make(chan struct{})

	// result channels used to communicate result information from the routing blocks
	// and step executors
	routingResultCh := make(chan routeResult)
	stepResultCh := make(chan stepResult)
	// error channel on which all failed targets will be forwarded
	targetErrCh := make(chan *target.Result)

	routeIn = make(chan *target.Target)

	// output channel of the pipeline
	out = make(chan *target.Target)

	for position, testStepBundle := range p.bundles {

		// Input and output channels for the Step
		stepInCh := make(chan *target.Target)
		stepOutCh := make(chan *target.Result)
		routeOut = make(chan *target.Target)

		if position == 0 {
			in = routeIn
		}

		stepChannels := stepCh{stepIn: stepInCh, stepOut: stepOutCh}
		routingChannels := routingCh{routeIn: routeIn, routeOut: routeOut, stepIn: stepInCh, stepOut: stepOutCh, targetErr: targetErrCh}

		// Build the Header that the the Step will be using for emitting events
		Header := testevent.Header{JobID: p.jobID, RunID: p.runID, TestName: p.test.Name, StepLabel: testStepBundle.StepLabel}
		ev := storage.NewTestEventEmitterFetcher(Header)

		router := newRouter(p.log, testStepBundle, routingChannels, ev, p.timeouts)
		go router.route(cancelInternalCh, pauseInternalCh, routingResultCh)
		go p.runStep(cancelInternalCh, pauseInternalCh, p.jobID, p.runID, testStepBundle, stepChannels, stepResultCh, ev)

		// The input of the next routing block is the output of the current routing block
		routeIn = routeOut
	}

	// forward all failed targets coming out from targetErrCh and last routOut channel
	// to the out channel of the pipeline
	go func() {
		for e := range targetErrCh {
			out <- e.Target
		}
	}()
	go func() {
		for t := range routeOut {
			out <- t
		}
	}()

	p.ctrlChannels = &pipelineCtrlCh{
		routingResultCh:  routingResultCh,
		stepResultCh:     stepResultCh,
		targetErrCh:      targetErrCh,
		cancelInternalCh: cancelInternalCh,
		pauseInternalCh:  pauseInternalCh,
	}
	return
}

// run is a blocking method which executes the pipeline until successful or failed termination
func (p *pipeline) run(cancel, pause <-chan struct{}) error {

	log := logging.AddField(p.log, "phase", "pipeline_run")

	log.Debugf("run")
	if p.ctrlChannels == nil {
		p.log.Panicf("pipeline is not initialized, control channels are not available")
	}

	log.Infof("running pipeline")
	var (
		waitError                           error
		cancellationAsserted, pauseAsserted bool
	)

	errWaitCh := make(chan error)
	cancelWaitCh := make(chan struct{})

	go func() {
		errWaitCh <- p.waitStepsAndRouting(cancelWaitCh, p.ctrlChannels.routingResultCh, p.ctrlChannels.stepResultCh, false)
	}()

	select {
	case waitError = <-errWaitCh:
		log.Debugf("waitError: %v", waitError)
		if waitError != nil {
			log.Debugf("waitStepsAndRouting returned with an error (%v), asserting cancellation.", waitError)
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
		log.Debugf("cancellation or pause asserted")
		close(cancelWaitCh)
		<-errWaitCh
	}

	if cancellationAsserted || pauseAsserted || waitError != nil {
		// we need to wait again for the pipeline to shutdown, this time with a timeout set externally
		log.Debugf("waiting for final termination")
		terminateWaitCh := make(chan struct{})
		waitCh := make(chan error)
		go func() {
			waitCh <- p.waitStepsAndRouting(terminateWaitCh, p.ctrlChannels.routingResultCh, p.ctrlChannels.stepResultCh, true)
		}()
		select {
		case <-time.After(p.timeouts.ShutdownTimeout):
			close(terminateWaitCh)
			<-waitCh
			waitError = fmt.Errorf("pipeline did not shutdown after timeout")
		case <-waitCh:
		}
		close(waitCh)
	}

	// close output channels only if termination was fully successfully
	// this could be more accurate and close as much as possible, knowing that
	// we are likely to leak something anyway
	if waitError == nil {
		close(p.ctrlChannels.routingResultCh)
		close(p.ctrlChannels.stepResultCh)
		close(p.ctrlChannels.targetErrCh)
	}
	log.Debugf("pipeline completed (err: %v)", waitError)
	return waitError
}

func newPipeline(log *logrus.Entry, bundles []test.StepBundle, test *test.Test, jobID types.JobID, runID types.RunID, timeouts TestRunnerTimeouts) *pipeline {
	p := pipeline{log: log, bundles: bundles, jobID: jobID, runID: runID, test: test, timeouts: timeouts}
	p.state = NewState()
	return &p
}
