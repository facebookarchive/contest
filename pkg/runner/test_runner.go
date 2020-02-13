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
)

var log = logging.GetLogger("pkg/test")

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
	bundle test.TestStepBundle
	err    error
}

// completionCh represents multiple result channels that the TestRunner consumes to
// collect results from routing blocks, TestSteps and Targets completing the test
type completionCh struct {
	routingResultCh <-chan routeResult
	stepResultCh    <-chan stepResult
	targetOut       <-chan *target.Target
	targetErr       <-chan cerrors.TargetError
}

// TestRunner is the main runner of TestSteps in ConTest. `results` collects
// the results of the run. It is not safe to access `results` concurrently.
type TestRunner struct {
	state    *RunnerState
	timeouts TestRunnerTimeouts
}

// WriteTargetErrorTimeout writes a TargetError object to a TargetError channel with timeout
func (tr *TestRunner) WriteTargetErrorTimeout(terminate <-chan struct{}, ch chan<- cerrors.TargetError, targetError cerrors.TargetError, timeout time.Duration) error {
	select {
	case <-terminate:
	case ch <- targetError:
	case <-time.After(timeout):
		return fmt.Errorf("timeout while writing targetError %+v", targetError)
	}
	return nil
}

// WriteTargetTimeout writes a Target object to a Target channel with timeout
func (tr *TestRunner) WriteTargetTimeout(terminate <-chan struct{}, ch chan<- *target.Target, target *target.Target, timeout time.Duration) error {
	select {
	case <-terminate:
	case ch <- target:
	case <-time.After(timeout):
		return fmt.Errorf("timeout while writing target %+v", target)
	}
	return nil
}

// InjectTarget attempts to deliver a Target on the input channel of a TestStep,
// returning the result of the operation on the result channel wrapped in the
// injectionCh argument
func (tr *TestRunner) InjectTarget(terminate <-chan struct{}, target *target.Target, ch injectionCh, wg *sync.WaitGroup) {
	defer wg.Done()

	err := tr.WriteTargetTimeout(terminate, ch.stepIn, target, tr.timeouts.StepInjectTimeout)

	select {
	case <-terminate:
	case ch.resultCh <- injectionResult{target: target, err: err}:
	case <-time.After(tr.timeouts.MessageTimeout):
		log.Panic("could not communicate back with ConTest")
	}
}

// Route implements the routing block associated with a TestStep that routes Targets
// into and out of a TestStep. It performs the following actions:
// * Consumes targets in input from the previous routing block
// * Asynchronously injects targets into the associated TestStep
// * Consumes targets in output from the associated TestStep
// * Asynchronously forwards targets to the following routing block
func (tr *TestRunner) Route(terminateRoute <-chan struct{}, bundle test.TestStepBundle, routingCh routingCh, resultCh chan<- routeResult, ev testevent.EmitterFetcher) {

	terminateInjection := make(chan struct{})

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
		select {
		case <-terminateRoute:
			err = fmt.Errorf("termination requested")
			break
		case injectionResult := <-injectResultCh:
			ingressTarget[pendingTarget] = time.Now()
			pendingTarget = nil
			if injectionResult.err != nil {
				err = fmt.Errorf("routing failed while injecting a target: %v", injectionResult.err)
				targetInErrEv := testevent.Data{EventName: target.EventTargetInErr, Target: injectionResult.target}
				if err := ev.Emit(targetInErrEv); err != nil {
					log.Warningf("Could not emit %v event for Target: %v", targetInErrEv, *injectionResult.target)
				}
				break
			}
			targetInEv := testevent.Data{EventName: target.EventTargetIn, TestStepIndex: bundle.TestStepIndex, Target: injectionResult.target}
			if err := ev.Emit(targetInEv); err != nil {
				log.Warningf("Could not emit %v event for Target: %v", targetInEv, *injectionResult.target)
			}
			if targets.Len() > 0 {
				pendingTarget = targets.Back().Value.(*target.Target)
				targets.Remove(targets.Back())
				injectionWg.Add(1)
				go tr.InjectTarget(terminateInjection, pendingTarget, injectionChannels, &injectionWg)
			}
		case t, chanIsOpen := <-tRouteIn:
			if !chanIsOpen {
				// The previous routing block has closed our input channel, signaling that
				// no more Targets will come through. Block reading from this channel
				tRouteIn = nil
			} else {
				// Buffer the target and check if there is already an injection in progress.
				// If so, pending targets will be dequeued only at the next result available
				// on `injectResultCh`.
				targets.PushFront(t)
				if pendingTarget == nil {
					pendingTarget = targets.Back().Value.(*target.Target)
					targets.Remove(targets.Back())
					injectionWg.Add(1)
					go tr.InjectTarget(terminateInjection, pendingTarget, injectionChannels, &injectionWg)
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
				// Emit an event signaling that the target has lef the TestStep
				targetOutEv := testevent.Data{EventName: target.EventTargetOut, TestStepIndex: bundle.TestStepIndex, Target: t}
				if err := ev.Emit(targetOutEv); err != nil {
					log.Warningf("Could not emit %v event for Target: %v", targetOutEv, *t)
				}
				// Register egress time and forward target to the next routing block
				egressTarget[t] = time.Now()
				if err := tr.WriteTargetTimeout(terminateRoute, routingCh.routeOut, t, tr.timeouts.MessageTimeout); err != nil {
					log.Panicf("step %s: could not forward target to the TestRunner: %+v", bundle.TestStepLabel, err)
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
				// Emit an event signaling that the target has lef the TestStep with an error
				payload := json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, targetError.Err))
				targetErrEv := testevent.Data{EventName: target.EventTargetErr, Target: targetError.Target, TestStepIndex: bundle.TestStepIndex, Payload: &payload}
				if err := ev.Emit(targetErrEv); err != nil {
					log.Warningf("Could not emit %v event for Target: %v", targetErrEv, *targetError.Target)
				}
				// Register egress time and forward the failing target to the TestRunner
				egressTarget[targetError.Target] = time.Now()
				if err := tr.WriteTargetErrorTimeout(terminateRoute, routingCh.targetErr, targetError, tr.timeouts.MessageTimeout); err != nil {
					log.Panicf("step %s: could not forward target to the TestRunner: %+v", bundle.TestStepLabel, err)
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
	// have injected have been returned by the TestStep.
	if err == nil {
		if len(ingressTarget) != len(egressTarget) {
			err = fmt.Errorf("step %s completed but did not return all injected Targets", bundle.TestStepLabel)
		} else {
			// Routing terminated without error. We might have been asked to terminate
			// from outside, or we might have terminated successfully. Close routing
			// output channel to signal to the next routing block that no more targets
			// will come through only in the latter case.
			select {
			case <-terminateRoute:
			default:
				close(routingCh.routeOut)
			}
		}
	}

	// Signal termination to the injection routines regardless of the result of the
	// routing. If the routing completed successfully, this is a no-op
	close(terminateInjection)

	// If there is an injection goroutine running, wait for it to terminate, as we
	// might have gotten here after a cancellation signal.
	injectionWg.Wait()

	// Send the result to the TestRunner, which is guaranteed to be listening. If
	// the TestRunner is not responsive before `MessageTimeout`, we are either running
	// on a system under heavy load which makes the runtime unable to properly schedule
	// goroutines, or we are hitting a bug.
	select {
	case resultCh <- routeResult{bundle: bundle, err: err}:
	case <-time.After(tr.timeouts.MessageTimeout):
		log.Panicf("could not send routing block result for step %s", bundle.TestStepLabel)
	}
}

// RunTestStep runs synchronously a TestStep and peforms sanity checks on the status
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
func (tr *TestRunner) RunTestStep(cancel, pause <-chan struct{}, bundle test.TestStepBundle, stepCh stepCh, resultCh chan<- stepResult, ev testevent.EmitterFetcher) {

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("step %s paniced (%v): %s", bundle.TestStepLabel, r, debug.Stack())
			select {
			case resultCh <- stepResult{bundle: bundle, err: err}:
			case <-time.After(tr.timeouts.MessageTimeout):
				log.Warningf("sending error back from TestStep runner timed out after %v. Error was: %v", tr.timeouts.MessageTimeout, err)
				return
			}
			return
		}
	}()

	// Run the TestStep and defer to the return path the handling of panic
	// conditions. If multiple error conditions occur, send downstream only
	// the first error encountered.
	channels := test.TestStepChannels{
		In:  stepCh.stepIn,
		Out: stepCh.stepOut,
		Err: stepCh.stepErr,
	}
	err := bundle.TestStep.Run(cancel, pause, channels, bundle.Parameters, ev)

	var (
		cancellationAsserted bool
		pauseAsserted        bool
	)
	// Check if we are shutting down. If so, do not perform sanity checks on the
	// channels but return immediately as the TestStep itself probably returned
	// because it honored the termination signal.
	select {

	case <-cancel:
		cancellationAsserted = true
		if err == nil {
			err = fmt.Errorf("test step cancelled")
		}
	case <-pause:
		pauseAsserted = true
	default:
	}

	if cancellationAsserted || pauseAsserted {
		select {
		case resultCh <- stepResult{bundle: bundle, err: err}:
		case <-time.After(tr.timeouts.MessageTimeout):
			log.Warningf("sending error back from TestStep runner timed out after %v. Error was: %v", tr.timeouts.MessageTimeout, err)
		}
		return
	}

	// If the TestStep has crashed with an error, return immediately the result to the TestRunner
	// which in turn will issue a cancellation signal to the pipeline
	if err != nil {
		select {
		case resultCh <- stepResult{bundle: bundle, err: err}:
		case <-time.After(tr.timeouts.MessageTimeout):
			log.Warningf("sending error back from TestStep runner timed out after %v. Error was: %v", tr.timeouts.MessageTimeout, err)
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
			err = fmt.Errorf("step returned, but input channel is not closed (api violation)")
		}
	default:
		// stepCh.stepIn is not closed, and a read operation would block. The TestStep
		// does not comply with the API (see above).
		if err == nil {
			err = fmt.Errorf("step returned, but input channel is not closed (api violation)")
		}
	}

	select {
	case _, ok := <-stepCh.stepOut:
		if !ok {
			// stepOutCh has been closed. This is a violation of the API. Record the error
			// if no other error condition has been seen.
			if err == nil {
				err = &cerrors.ErrTestStepClosedChannels{StepName: bundle.TestStep.Name()}
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
				err = &cerrors.ErrTestStepClosedChannels{StepName: bundle.TestStep.Name()}
			}
		}
	default:
		// stepCh.stepErr is open. Flag it for closure to signal to the routing subsystem that
		// no more Targets will go through from this channel.
		close(stepCh.stepErr)
	}

	select {
	case resultCh <- stepResult{bundle: bundle, err: err}:
	case <-time.After(tr.timeouts.MessageTimeout):
		log.Warningf("sending error back from TestStep runner timed out after %v. Error was: %v", tr.timeouts.MessageTimeout, err)
		return
	}
}

// WaitTestStep reads results coming from result channels until `StepShutdownTimeout`
// occurs or an error is encountered. It then checks whether TestSteps and routing
// blocks have all returned correctly. If not, it returns an error.
func (tr *TestRunner) WaitTestStep(ch completionCh, bundles []test.TestStepBundle) error {
	var err error

wait_test_step:
	for {
		select {
		case <-time.After(tr.timeouts.StepShutdownTimeout):
			break wait_test_step
		case res := <-ch.routingResultCh:
			err = res.err
			tr.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-ch.stepResultCh:
			err = res.err
			tr.state.SetStep(res.bundle.TestStepLabel, res.err)
		}

		if err != nil {
			return err
		}
		stepsCompleted := len(tr.state.CompletedSteps()) == len(bundles)
		routingCompleted := len(tr.state.CompletedRouting()) == len(bundles)
		if stepsCompleted && routingCompleted {
			break wait_test_step
		}
	}

	incompleteSteps := tr.state.IncompleteSteps(bundles)
	if len(incompleteSteps) > 0 {
		err = &cerrors.ErrTestStepsNeverReturned{StepNames: incompleteSteps}
	} else if len(tr.state.CompletedRouting()) != len(bundles) {
		err = fmt.Errorf("not all routing completed")
	}
	return err
}

// WaitPipelineTermination reads results coming from result channels waiting
// for the pipeline to completely shutdown before `ShutdownTimeout` occurs. A
// "complete shutdown" means that all TestSteps and routing blocks have sent
// downstream their results.
func (tr *TestRunner) WaitPipelineTermination(ch completionCh, bundles []test.TestStepBundle) error {
	log.Printf("Waiting for pipeline to terminate")
	for {
		select {
		case <-time.After(tr.timeouts.ShutdownTimeout):
			incompleteSteps := tr.state.IncompleteSteps(bundles)
			if len(incompleteSteps) > 0 {
				return &cerrors.ErrTestStepsNeverReturned{StepNames: tr.state.IncompleteSteps(bundles)}
			}
			return fmt.Errorf("pipeline did not return but all test steps completed")
		case res := <-ch.routingResultCh:
			tr.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-ch.stepResultCh:
			tr.state.SetStep(res.bundle.TestStepLabel, res.err)
		}

		stepsCompleted := len(tr.state.CompletedSteps()) == len(bundles)
		routingCompleted := len(tr.state.CompletedRouting()) == len(bundles)
		if stepsCompleted && routingCompleted {
			return nil
		}
	}
}

// WaitPipelineCompletion reads results coming from results channels until all Targets
// have completed or an error occurs. If all Targets complete successfully, it checks
// whether TestSteps and routing blocks have completed as well. If not, returns an
// error. Termination is signalled via terminate channel.
func (tr *TestRunner) WaitPipelineCompletion(terminate <-chan struct{}, ch completionCh, bundles []test.TestStepBundle, targets []*target.Target) error {
	var err error
	for {
		if len(tr.state.CompletedTargets()) == len(targets) {
			break
		}
		if err != nil {
			return err
		}
		select {
		case <-terminate:
			// When termination is signaled just stop WaitPipelineCompletion. It is up
			// to the caller to decide how to further handle pipeline termination.
			return nil
		case res := <-ch.routingResultCh:
			err = res.err
			tr.state.SetRouting(res.bundle.TestStepLabel, res.err)
		case res := <-ch.stepResultCh:
			err = res.err
			tr.state.SetStep(res.bundle.TestStepLabel, res.err)
		case targetErr := <-ch.targetErr:
			tr.state.SetTarget(targetErr.Target, targetErr.Err)
		case target, chanIsOpen := <-ch.targetOut:
			if !chanIsOpen {
				if len(tr.state.CompletedTargets()) != len(targets) {
					err = fmt.Errorf("not all targets completed, but output channel is closed")
				}
			} else {
				tr.state.SetTarget(target, nil)
			}
		}
	}

	// The test run completed, we have collected all Targets. TestSteps might have already
	// closed `ch.out`, in which case the pipeline terminated correctly (channels are closed
	// in a "domino" sequence, so seeing the last channel closed indicates that the
	// sequence of close operations has completed). If `ch.out` is still open,
	// there are still TestSteps that might have not returned. Wait for all
	// TestSteps to complete or `StepShutdownTimeout` to occurr.
	log.Printf("Waiting for all TestSteps to complete")
	err = tr.WaitTestStep(ch, bundles)
	return err
}

// Run implements the main logic of the TestRunner, i.e. the instantiation and
// connection of the TestSteps, routing blocks and pipeline runner.
func (tr *TestRunner) Run(cancel, pause <-chan struct{}, t *test.Test, targets []*target.Target, jobID types.JobID) (*test.TestResult, error) {
	testStepBundles := t.TestStepsBundles
	if len(testStepBundles) == 0 {
		return nil, fmt.Errorf("no steps to run for test")
	}

	var (
		cancellationAsserted bool
		pauseAsserted        bool
	)

	pauseTestStep := make(chan struct{})
	cancelTestStep := make(chan struct{})
	// termination channels are used to signal termination to injection and routing
	terminateWaitCompletion := make(chan struct{})
	terminateInjection := make(chan struct{})
	terminateRouting := make(chan struct{})

	// result channels used to communicate result information from the routing blocks
	// and step executors
	routingResultCh := make(chan routeResult)
	stepResultCh := make(chan stepResult)
	targetErrCh := make(chan cerrors.TargetError)

	var (
		routeIn  chan *target.Target
		routeOut chan *target.Target
	)

	for r, testStepBundle := range testStepBundles {
		// Input and output channels for the TestStep
		stepInCh := make(chan *target.Target)
		stepOutCh := make(chan *target.Target)
		stepErrCh := make(chan cerrors.TargetError)

		// Output of the current routing block
		routeOut = make(chan *target.Target)

		// First step of the pipeline
		if r == 0 {
			routeIn = make(chan *target.Target)
			// Spawn a goroutine which injects Targets into the first routing block
			go func(terminate <-chan struct{}, inputChannel chan<- *target.Target) {
				defer close(inputChannel)
				for _, target := range targets {
					if err := tr.WriteTargetTimeout(terminate, inputChannel, target, tr.timeouts.MessageTimeout); err != nil {
						log.Panic(fmt.Sprintf("could not inject target %+v into first routing block: %+v", target, err))
					}
				}
			}(terminateInjection, routeIn)
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
			JobID:         jobID,
			TestName:      t.Name,
			TestStepLabel: testStepBundle.TestStepLabel,
		}
		ev := storage.NewTestEventEmitterFetcher(Header)
		go tr.Route(terminateRouting, testStepBundle, routingChannels, routingResultCh, ev)
		go tr.RunTestStep(cancelTestStep, pauseTestStep, testStepBundle, stepChannels, stepResultCh, ev)
		// The input of the next routing block is the output of the current routing block
		routeIn = routeOut
	}

	var (
		completionError  error
		terminationError error
	)

	completionChannels := completionCh{
		routingResultCh: routingResultCh,
		stepResultCh:    stepResultCh,
		targetErr:       targetErrCh,
		targetOut:       routeOut,
	}

	// errCh collects errors coming from the routines which wait for the Test to complete
	errCh := make(chan error)

	// Wait for the pipeline to complete. If an error occurrs, cancel all TestSteps
	// and routing blocks and wait again for completion until shutdown timeout occurrs.
	log.Printf("TestRunner: waiting for test to complete")

	go func() {
		errCh <- tr.WaitPipelineCompletion(terminateWaitCompletion, completionChannels, testStepBundles, targets)
	}()

	select {
	case completionError = <-errCh:
	case <-cancel:
		close(terminateWaitCompletion)
		cancellationAsserted = true
		completionError = <-errCh
	case <-pause:
		close(terminateWaitCompletion)
		pauseAsserted = true
		completionError = <-errCh
	}

	if completionError != nil || cancellationAsserted {
		// If the Test has encountered an error or cancellation has been asserted,
		// terminate routing and injection and propagate the cancel signal to the
		// TestStep(s)
		cancellationAsserted = true
		if completionError != nil {
			log.Printf("TestRunner: test failed to complete: %+v. Forcing cancellation.", completionError)
		} else {
			log.Printf("TestRunner: cancellation was asserted")
		}
		close(cancelTestStep)
		close(terminateInjection)
		close(terminateRouting)
	}
	if pauseAsserted {
		// If pause signal has been asserted, terminate routing and injection and
		// propagate the pause signal to the TestStep(s).
		log.Printf("TestRunner has received pause request")
		close(pauseTestStep)
		close(terminateInjection)
		close(terminateRouting)
	}

	// If either cancellation or pause have been asserted, we need to wait for the
	// pipeline to terminate
	if cancellationAsserted || pauseAsserted {
		go func() {
			errCh <- tr.WaitPipelineTermination(completionChannels, testStepBundles)
		}()
		signal := "cancellation"
		if pauseAsserted {
			signal = "pause"
		}
		terminationError = <-errCh
		if terminationError != nil {
			log.Printf("TestRunner: test did not terminate correctly after %s signal: %+v", signal, terminationError)
		} else {
			log.Printf("TestRunner: test terminated correctly after %s signal", signal)
		}
	} else {
		log.Printf("TestRunner completed")
	}

	if completionError != nil {
		return nil, completionError
	}

	testResult := test.NewTestResult(jobID)
	for k, v := range tr.state.CompletedTargets() {
		testResult.SetTarget(k, v)
	}
	return &testResult, terminationError
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
		state: NewRunnerState(),
	}
}

// NewTestRunnerWithTimeouts initializes and returns a new TestRunner object with
// custom timeouts
func NewTestRunnerWithTimeouts(timeouts TestRunnerTimeouts) TestRunner {
	return TestRunner{timeouts: timeouts, state: NewRunnerState()}
}
