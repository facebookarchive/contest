// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"fmt"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/teststeps/example"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var ctx = logrusctx.NewContext(logger.LevelDebug)

var testSteps = map[string]test.TestStepFactory{
	example.Name: example.New,
}

var testStepsEvents = map[string][]event.Name{
	example.Name: example.Events,
}

type TestRunnerSuite struct {
	suite.Suite
	routingChannels routingCh
	router          *stepRouter

	// the following channels are the same channels wrapped within `routingChannels`,
	// but used in the opposite direction compared to the routing block. When the
	// routing block reads results, the corresponding channel of the test suite has
	// write direction, when the routing block writes results, the corresponding channel
	// of the test suite has read direction
	routeInCh  chan<- *target.Target
	routeOutCh <-chan *target.Target

	stepInCh  <-chan *target.Target
	stepOutCh chan<- *target.Target
	stepErrCh chan<- cerrors.TargetError

	targetErrCh <-chan cerrors.TargetError
}

func (suite *TestRunnerSuite) SetupTest() {

	pluginRegistry := pluginregistry.NewPluginRegistry(ctx)
	// Setup the PluginRegistry by registering TestSteps
	for name, tsfactory := range testSteps {
		if _, ok := testStepsEvents[name]; !ok {
			suite.T().Errorf("test step %s does not define any associated event", name)
		}
		err := pluginRegistry.RegisterTestStep(name, tsfactory, testStepsEvents[name])
		require.NoError(suite.T(), err)
	}
	ts1, err := pluginRegistry.NewTestStep("Example")
	require.NoError(suite.T(), err)

	params := make(test.TestStepParameters)
	bundle := test.TestStepBundle{TestStep: ts1, TestStepLabel: "FirstStage", Parameters: params}

	routeInCh := make(chan *target.Target)
	routeOutCh := make(chan *target.Target)
	stepInCh := make(chan *target.Target)
	stepOutCh := make(chan *target.Target)
	stepErrCh := make(chan cerrors.TargetError)
	targetErrCh := make(chan cerrors.TargetError)

	suite.routeInCh = routeInCh
	suite.routeOutCh = routeOutCh
	suite.stepOutCh = stepOutCh
	suite.stepInCh = stepInCh
	suite.stepErrCh = stepErrCh
	suite.targetErrCh = targetErrCh

	suite.routingChannels = routingCh{
		routeIn:   routeInCh,
		routeOut:  routeOutCh,
		stepIn:    stepInCh,
		stepErr:   stepErrCh,
		stepOut:   stepOutCh,
		targetErr: targetErrCh,
	}

	s, err := memory.New()
	require.NoError(suite.T(), err)
	err = storage.SetStorage(s)
	require.NoError(suite.T(), err)

	header := testevent.Header{
		JobID:         1,
		RunID:         1,
		TestName:      "TestRunnerSuite",
		TestStepLabel: "TestRunnerSuite",
	}
	ev := storage.NewTestEventEmitterFetcher(header)

	timeouts := TestRunnerTimeouts{
		StepInjectTimeout:   30 * time.Second,
		MessageTimeout:      25 * time.Second,
		ShutdownTimeout:     5 * time.Second,
		StepShutdownTimeout: 5 * time.Second,
	}

	suite.router = newStepRouter(bundle, suite.routingChannels, ev, timeouts)
}

func (suite *TestRunnerSuite) TestRouteInRoutesAllTargets() {

	// test that all targets are routed to step input channel without delay, in order
	targets := []*target.Target{
		{ID: "001", FQDN: "host001.facebook.com"},
		{ID: "002", FQDN: "host002.facebook.com"},
		{ID: "003", FQDN: "host003.facebook.com"},
	}

	routeInResult := make(chan error)
	stepInResult := make(chan error)

	go func() {
		// start routing
		_, _ = suite.router.routeIn(ctx)
	}()

	// inject targets
	go func() {
		defer close(suite.routeInCh)
		for _, target := range targets {
			select {
			case <-time.After(2 * time.Second):
				routeInResult <- fmt.Errorf("target should be accepted by routeIn block within timeout")
				return
			case suite.routeInCh <- target:
			}
		}
		routeInResult <- nil
	}()

	// mimic the test step consuming targets in input
	go func() {
		numTargets := 0
		for {
			select {
			case t, ok := <-suite.stepInCh:
				if !ok {
					if numTargets != len(targets) {
						stepInResult <- fmt.Errorf("not all targets received by teste step")
					} else {
						stepInResult <- nil
					}
					return
				}
				if numTargets+1 > len(targets) {
					stepInResult <- fmt.Errorf("more targets returned than injected")
					return
				}
				if t != targets[numTargets] {
					stepInResult <- fmt.Errorf("targets returned in wrong order")
					return
				}
				numTargets++
			case <-time.After(2 * time.Second):
				stepInResult <- fmt.Errorf("expected target on channel within timeout")
			}
		}
	}()

	for {
		select {
		case <-time.After(5 * time.Second):
			suite.T().Errorf("test should return within timeout")
		case err := <-stepInResult:
			stepInResult = nil
			if err != nil {
				suite.T().Errorf("step in returned error: %v", err)
			}
		case err := <-routeInResult:
			routeInResult = nil
			if err != nil {
				suite.T().Errorf("route in returned error: %v", err)
			}
		}
		if stepInResult == nil && routeInResult == nil {
			return
		}
	}
}

func (suite *TestRunnerSuite) TestRouteOutRoutesAllSuccessfulTargets() {

	// test that all targets are routed in output from a test step are received by
	// the routing logic and forwarded to the next routing block
	targets := []*target.Target{
		{ID: "001", FQDN: "host001.facebook.com"},
		{ID: "002", FQDN: "host002.facebook.com"},
		{ID: "003", FQDN: "host003.facebook.com"},
	}

	go func() {
		// start routing
		_, _ = suite.router.routeOut(ctx)
	}()

	stepResult := make(chan error)
	routeResult := make(chan error)

	// mimic the test step returning targets on the output channel. This test exercise
	// the path where all targets complete successfully
	go func() {
		// it's expected that the step closes both channels
		defer close(suite.stepOutCh)
		defer close(suite.stepErrCh)
		for _, target := range targets {
			select {
			case <-time.After(2 * time.Second):
				stepResult <- fmt.Errorf("target should be accepted by routing block within timeout")
				return
			case suite.stepOutCh <- target:
			}
		}
		stepResult <- nil
	}()

	// mimic the next routing block and the pipeline reading from the routeOut channel and the targetErr
	// channel. We don't expect any target coming through targetErr, we expect all targets to be successful
	go func() {
		numTargets := 0
		for {
			select {
			case _, ok := <-suite.targetErrCh:
				if !ok {
					suite.targetErrCh = nil
				} else {
					routeResult <- fmt.Errorf("no targets expected on the error channel")
					return
				}
			case t, ok := <-suite.routeOutCh:
				if !ok {
					if numTargets != len(targets) {
						routeResult <- fmt.Errorf("not all targets have been returned")
					} else {
						routeResult <- nil
					}
					return
				}
				if numTargets+1 > len(targets) {
					routeResult <- fmt.Errorf("more targets returned than injected")
					return
				}
				if t.ID != targets[numTargets].ID {
					routeResult <- fmt.Errorf("targets returned in wrong order")
					return
				}
				numTargets++
			case <-time.After(2 * time.Second):
				routeResult <- fmt.Errorf("expected target on channel within timeout")
				return
			}
		}
	}()

	for {
		select {
		case <-time.After(5 * time.Second):
			suite.T().Errorf("test should return within timeout")
		case err := <-stepResult:
			stepResult = nil
			if err != nil {
				suite.T().Errorf("step output returned error: %v", err)
			}
		case err := <-routeResult:
			routeResult = nil
			if err != nil {
				suite.T().Errorf("router returned error: %v", err)
			}
		}
		if stepResult == nil && routeResult == nil {
			return
		}
	}
}

func (suite *TestRunnerSuite) TestRouteOutRoutesAllFailedTargets() {

	// test that all targets are routed in output from a test step are received by
	// the routing logic and forwarded to the next routing block
	targets := []*target.Target{
		{ID: "001", FQDN: "host001.facebook.com"},
		{ID: "002", FQDN: "host002.facebook.com"},
		{ID: "003", FQDN: "host003.facebook.com"},
	}

	go func() {
		// start routing
		_, _ = suite.router.routeOut(ctx)
	}()

	stepResult := make(chan error)
	routeResult := make(chan error)

	// mimic the test step returning targets on the output channel. This test exercise
	// the path where all targets complete successfully
	go func() {
		// it's expected that the step closes both channels
		defer close(suite.stepOutCh)
		defer close(suite.stepErrCh)
		for _, target := range targets {
			targetErr := cerrors.TargetError{Target: target, Err: fmt.Errorf("test error")}
			select {
			case <-time.After(2 * time.Second):
				stepResult <- fmt.Errorf("target should be accepted by routing block within timeout")
				return
			case suite.stepErrCh <- targetErr:
			}
		}
		stepResult <- nil
	}()

	// mimic the next routing block and the pipeline reading from the routeOut channel and the targetErr
	// channel. We don't expect any target coming through targetErr, we expect all targets to be successful
	go func() {
		numTargets := 0
		for {
			select {
			case targetErr, ok := <-suite.targetErrCh:
				if !ok {
					routeResult <- fmt.Errorf("target error channel should not be closed by routing block")
					return
				}
				if targetErr.Err == nil {
					routeResult <- fmt.Errorf("expected error associated to the target")
					return
				}
				if numTargets+1 > len(targets) {
					routeResult <- fmt.Errorf("more targets returned than injected")
					return
				}
				if targetErr.Target.ID != targets[numTargets].ID {
					routeResult <- fmt.Errorf("targets returned in wrong order")
					return
				}

				numTargets++
				if numTargets == len(targets) {
					routeResult <- nil
					return
				}
			case _, ok := <-suite.routeOutCh:
				if !ok {
					suite.routeOutCh = nil
				} else {
					routeResult <- fmt.Errorf("no targets expected on the output channel")
					return
				}
			case <-time.After(2 * time.Second):
				routeResult <- fmt.Errorf("expected target on channel within timeout")
				return
			}
		}
	}()

	for {
		select {
		case <-time.After(5 * time.Second):
			suite.T().Errorf("test should return within timeout")
		case err := <-stepResult:
			stepResult = nil
			if err != nil {
				suite.T().Errorf("step output routine returned error: %v", err)
			}
		case err := <-routeResult:
			routeResult = nil
			if err != nil {
				suite.T().Errorf("router returned error: %v", err)
			}
		}
		if stepResult == nil && routeResult == nil {
			return
		}
	}
}

func TestTestRunnerSuite(t *testing.T) {
	TestRunnerSuite := &TestRunnerSuite{}
	suite.Run(t, TestRunnerSuite)
}
