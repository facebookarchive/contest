// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"fmt"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/logging"
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
// There is a routing block for each Step of the pipeline, which is responsible for
// the following actions:
// * Targets in egress from the previous routing block are injected into the
// current Step
// * Targets in egress from the current Step are injected into the next
// routing block
type routingCh struct {
	// routeIn and routeOut connect the routing block to other routing blocks
	routeIn  <-chan *target.Target
	routeOut chan<- *target.Target
	// Channels that connect the routing block to the Step
	stepIn  chan<- *target.Target
	stepOut <-chan *target.Target
	stepErr <-chan cerrors.TargetError
	// targetErr connects the routing block directly to the TestRunner. Failing
	// targets are acquired by the TestRunner via this channel
	targetErr chan<- cerrors.TargetError
}

// stepCh represents a set of bidirectional channels that a Step and its associated
// routing block use to communicate. The TestRunner forces the direction of each
// channel when connecting the Step to the routing block.
type stepCh struct {
	stepIn  chan *target.Target
	stepOut chan *target.Target
	stepErr chan cerrors.TargetError
}

type injectionCh struct {
	stepIn   chan<- *target.Target
	resultCh chan injectionResult
}

// injectionResult represents the result of an injection goroutine
type injectionResult struct {
	target *target.Target
	err    error
}

// routeResult represents the result of routing block, possibly carrying error information
type routeResult struct {
	bundle test.StepBundle
	err    error
}

// stepResult represents the result of a Step, possibly carrying error information
type stepResult struct {
	jobID  types.JobID
	runID  types.RunID
	bundle test.StepBundle
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

// TestRunner is the main runner of Steps in ConTest. `results` collects
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
		w.log.Debugf("terminate requested while writing result for target target %+v", target)
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

func (tr *TestRunner) injectTargets(terminate <-chan struct{}, targets []*target.Target, writeChannel chan<- *target.Target, log *logrus.Entry) {
	writer := newTargetWriter(log, tr.timeouts)
	for _, target := range targets {
		log.Debugf("injecting %v", target)
		if err := writer.writeTimeout(terminate, writeChannel, target, tr.timeouts.MessageTimeout); err != nil {
			log.Panicf("could not inject target %+v into first routing block: %+v", target, err)
		}
		select {
		case <-terminate:
			log.Debugf("terminate requested while injecting targets from list")
			return
		default:
		}
	}
	log.Debugf("all targets have been injected")
}

// inject forwards targets from a read channel to a write channel, until the termination signal is asserted
// or the read channel is closed
func (tr *TestRunner) pipeChannels(terminate <-chan struct{}, readChannel <-chan *target.Target, writeChannel chan<- *target.Target, log *logrus.Entry) {
	defer close(writeChannel)
	for {
		select {
		case <-terminate:
			log.Debugf("terminate requested injecting from channel")
			return
		case t, ok := <-readChannel:
			if !ok {
				log.Debugf("pipe input channel closed, closing pipe output channel")
				return
			}
			log.Debugf("received target %v, injecting...", t)
			tr.injectTargets(terminate, []*target.Target{t}, writeChannel, log)
		}
	}
}

func (tr *TestRunner) waitTestRunner(cancel, pause <-chan struct{}, cancelInternal, pauseInternal chan struct{}, errTestCh, errCleanupCh chan error, completed <-chan *target.Target, log *logrus.Entry) error {

	var (
		errTest, errCleanup                 error
		testCompleted, cleanupCompleted     bool
		cancellationAsserted, pauseAsserted bool
	)

	for {
		select {
		case <-cancel:
			log.Debug("cancellation asserted, propagating internal cancellation signal")
			cancellationAsserted = true
			close(cancelInternal)
		case <-pause:
			log.Debug("pause asserted, propagating internal pause signal")
			pauseAsserted = true
			close(pauseInternal)
		case errTest = <-errTestCh:
			log.Debugf("test pipeline terminated")
			testCompleted = true
		case errCleanup = <-errCleanupCh:
			log.Debugf("cleanup pipeline terminated")
			cleanupCompleted = true
		case target, chanIsOpen := <-completed:
			log.Infof("test runner completed target: %v", target)
			if !chanIsOpen {
				completed = nil
			}
		}

		log.Debugf("cleanup completed: %v, test completed: %v", cleanupCompleted, testCompleted)
		if testCompleted && cleanupCompleted {
			err := newErrTestRunnerError(errTest, errCleanup)
			log.Debugf("test runner returning %v", err)
			return err
		}

		if errTest != nil || errCleanup != nil {
			cancellationAsserted = true
			close(cancelInternal)
		}

		if cancellationAsserted || pauseAsserted {
			if !cleanupCompleted {
				log.Debugf("cancellation asserted, waiting for cleanup to complete")
				errCleanup = <-errCleanupCh
			}
			if !testCompleted {
				log.Debugf("cancellation asserted, waiting for test to complete")
				errTest = <-errTestCh
			}
			err := newErrTestRunnerError(errTest, errCleanup)
			log.Debugf("test runner returning %v", err)
			return err
		}
	}

}

// Run implements the main logic of the TestRunner, i.e. the instantiation and
// connection of the Steps, routing blocks and pipeline runner.
func (tr *TestRunner) Run(cancel, pause <-chan struct{}, test *test.Test, targets []*target.Target, jobID types.JobID, runID types.RunID) error {

	// rootLog is propagated to all the subsystems of the pipeline
	rootLog := logging.GetLogger("pkg/runner")
	fields := make(map[string]interface{})
	fields["jobid"] = jobID
	fields["runid"] = runID
	rootLog = logging.AddFields(rootLog, fields)
	log := logging.AddField(rootLog, "phase", "run")

	if len(test.TestStepsBundles) == 0 {
		return fmt.Errorf("no steps to run for test")
	}

	// internal cancellation and pause signals for the pipelines
	cancelInternal := make(chan struct{})
	pauseInternal := make(chan struct{})

	errTestCh := make(chan error)
	errCleanupCh := make(chan error)

	// setup the test pipeline
	log.Infof("setting up test pipeline")
	testLog := logging.AddField(rootLog, "entity", "test_pipeline")
	testPipeline := newPipeline(testLog, test.TestStepsBundles, test, jobID, runID, tr.timeouts)
	inputTestPipelineCh := testPipeline.init(cancel, pause)

	completedFromTestCh := make(chan *target.Target)

	// completed represents the final channel where all completed targets are received.
	// If there is no cleanup pipeline configured for the test, this channel corresponds
	// to the output channel of the test pipeline. If instead there is a cleanup pipeline
	// configured, this channel corresponds the the output channel of the cleanup pipeline.
	completed := completedFromTestCh
	go func() {
		log.Infof("running test pipeline")
		errTestCh <- testPipeline.run(cancelInternal, pauseInternal, completedFromTestCh)
		close(completedFromTestCh)
	}()

	// inject targets into the step pipeline
	cancelInjectionCh := make(chan struct{})
	go func(terminate <-chan struct{}, writeChannel chan<- *target.Target) {
		defer close(writeChannel)
		log := logging.AddField(log, "step", "test_injection")
		tr.injectTargets(cancelInjectionCh, targets, writeChannel, log)
	}(cancelInjectionCh, inputTestPipelineCh)

	if len(test.CleanupStepsBundles) == 0 {
		// cleanup pipeline will not run
		go func() {
			errCleanupCh <- nil
		}()
		log.Warningf("no cleanup pipeline defined")
	} else {

		// setup the cleanup pipeline
		log.Infof("setting up cleanup pipeline")
		cleanupLog := logging.AddField(rootLog, "entity", "cleanup_pipeline")
		cleanupPipeline := newPipeline(cleanupLog, test.CleanupStepsBundles, test, jobID, runID, tr.timeouts)
		inputCleanupCh := cleanupPipeline.init(cancel, pause)

		// forward all targets coming out of the test pipeline into the cleanup pipeline
		go func(terminate <-chan struct{}, readChannel <-chan *target.Target, writeChannel chan<- *target.Target) {
			log := logging.AddField(log, "step", "cleanup_injection")
			tr.pipeChannels(terminate, readChannel, writeChannel, log)
		}(cancelInjectionCh, completedFromTestCh, inputCleanupCh)

		completedFromCleanup := make(chan *target.Target)
		go func() {
			log.Infof("running cleanup pipeline")
			errCleanupCh <- cleanupPipeline.run(cancelInternal, pauseInternal, completedFromCleanup)
		}()
		completed = completedFromCleanup
	}

	defer close(cancelInjectionCh)
	// Receive targets from the completed channel controlled by the pipeline, while
	// waiting for termination signals or fatal errors encountered while running
	// the pipeline.
	waitLog := logging.AddField(rootLog, "phase", "waitTestRunner")
	return tr.waitTestRunner(cancel, pause, cancelInternal, pauseInternal, errTestCh, errCleanupCh, completed, waitLog)
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
func (r *State) IncompleteSteps(bundles []test.StepBundle) []string {
	var incompleteSteps []string
	for _, bundle := range bundles {
		if _, ok := r.completedSteps[bundle.StepLabel]; !ok {
			incompleteSteps = append(incompleteSteps, bundle.StepLabel)
		}
	}
	return incompleteSteps
}
