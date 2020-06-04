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
