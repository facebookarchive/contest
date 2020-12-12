// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"fmt"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/teststeps/example"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var testSteps = map[string]test.StepFactory{
	example.Name: example.New,
}

var testStepsEvents = map[string][]event.Name{
	example.Name: example.Events,
}

type TestRunnerSuite struct {
	suite.Suite
	routingChannels routingCh
	router          *router

	// the following channels are the same channels wrapped within `routingChannels`,
	// but used in the opposite direction compared to the routing block. When the
	// routing block reads results, the corresponding channel of the test suite has
	// write direction, when the routing block writes results, the corresponding channel
	// of the test suite has read direction
	routeInCh  chan<- *target.Target
	routeOutCh <-chan *target.Target

	stepInCh  <-chan *target.Target
	stepOutCh chan<- *target.Result

	targetErrCh <-chan *target.Result
}

func (suite *TestRunnerSuite) SetupTest() {

	log := logging.GetLogger("TestRunnerSuite")

	pluginRegistry := pluginregistry.NewPluginRegistry()
	// Setup the PluginRegistry by registering Steps
	for name, tsfactory := range testSteps {
		if _, ok := testStepsEvents[name]; !ok {
			suite.T().Errorf("test step %s does not define any associated event", name)
		}
		err := pluginRegistry.RegisterStep(name, tsfactory, testStepsEvents[name])
		require.NoError(suite.T(), err)
	}
	ts1, err := pluginRegistry.NewStep("Example")
	require.NoError(suite.T(), err)

	params := make(test.StepParameters)
	bundle := test.StepBundle{Step: ts1, StepLabel: "FirstStage", Parameters: params}

	routeInCh := make(chan *target.Target)
	routeOutCh := make(chan *target.Target)
	stepInCh := make(chan *target.Target)
	stepOutCh := make(chan *target.Result)
	targetErrCh := make(chan *target.Result)

	suite.routeInCh = routeInCh
	suite.routeOutCh = routeOutCh
	suite.stepOutCh = stepOutCh
	suite.stepInCh = stepInCh
	suite.targetErrCh = targetErrCh

	suite.routingChannels = routingCh{
		routeIn:   routeInCh,
		routeOut:  routeOutCh,
		stepIn:    stepInCh,
		stepOut:   stepOutCh,
		targetErr: targetErrCh,
	}

	s, err := memory.New()
	require.NoError(suite.T(), err)
	storage.SetStorage(s)

	header := testevent.Header{
		JobID:     1,
		RunID:     1,
		TestName:  "TestRunnerSuite",
		StepLabel: "TestRunnerSuite",
	}
	ev := storage.NewTestEventEmitterFetcher(header)

	timeouts := TestRunnerTimeouts{
		StepInjectTimeout:   30 * time.Second,
		MessageTimeout:      5 * time.Second,
		ShutdownTimeout:     1 * time.Second,
		StepShutdownTimeout: 1 * time.Second,
	}

	suite.router = newRouter(log, bundle, suite.routingChannels, ev, timeouts)
}

func (suite *TestRunnerSuite) TestRouteInRoutesAllTargets() {

	// test that all targets are routed to step input channel without delay, in order
	targets := []*target.Target{
		&target.Target{Name: "host001", ID: "001", FQDN: "host001.facebook.com"},
		&target.Target{Name: "host002", ID: "002", FQDN: "host002.facebook.com"},
		&target.Target{Name: "host003", ID: "003", FQDN: "host003.facebook.com"},
	}

	routeInResult := make(chan error)
	stepInResult := make(chan error)
	terminate := make(chan struct{})

	go func() {
		// start routing
		_ = suite.router.routeIn(terminate)
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
			case _, ok := <-suite.stepInCh:
				var err error
				if !ok {
					if numTargets != len(targets) {
						err = fmt.Errorf("not all targets received by teste step")
					}
					stepInResult <- err
					return
				}
				if numTargets+1 > len(targets) {
					err = fmt.Errorf("more targets returned than injected")
				}
				if err != nil {
					stepInResult <- err
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
		&target.Target{Name: "host001", ID: "001", FQDN: "host001.facebook.com"},
		&target.Target{Name: "host002", ID: "002", FQDN: "host002.facebook.com"},
		&target.Target{Name: "host003", ID: "003", FQDN: "host003.facebook.com"},
	}

	terminate := make(chan struct{})

	go func() {
		// start routing
		_ = suite.router.routeOut(terminate)
	}()

	stepResult := make(chan error)
	routeResult := make(chan error)

	// mimic the test step returning targets on the output channel. This test exercise
	// the path where all targets complete successfully
	go func() {
		// it's expected that the step closes both channels
		defer close(suite.stepOutCh)
		for _, t := range targets {
			select {
			case <-time.After(2 * time.Second):
				stepResult <- fmt.Errorf("target should be accepted by routing block within timeout")
				return
			case suite.stepOutCh <- &target.Result{Target: t}:
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
			case _, ok := <-suite.routeOutCh:
				var err error
				if !ok {
					if numTargets != len(targets) {
						err = fmt.Errorf("not all targets have been returned")
					}
					routeResult <- err
					return
				}
				if numTargets+1 > len(targets) {
					err = fmt.Errorf("more targets returned than injected")
				}
				if err != nil {
					routeResult <- err
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
		&target.Target{Name: "host001", ID: "001", FQDN: "host001.facebook.com"},
		&target.Target{Name: "host002", ID: "002", FQDN: "host002.facebook.com"},
		&target.Target{Name: "host003", ID: "003", FQDN: "host003.facebook.com"},
	}

	terminate := make(chan struct{})

	go func() {
		// start routing
		_ = suite.router.routeOut(terminate)
	}()

	stepResult := make(chan error)
	routeResult := make(chan error)

	// mimic the test step returning targets on the output channel. This test exercise
	// the path where all targets complete successfully
	go func() {
		// it's expected that the step closes both channels
		defer close(suite.stepOutCh)
		for _, t := range targets {
			select {
			case <-time.After(2 * time.Second):
				stepResult <- fmt.Errorf("target should be accepted by routing block within timeout")
				return
			case suite.stepOutCh <- &target.Result{Target: t, Err: fmt.Errorf("test error")}:
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
				var err error
				if targetErr.Err == nil {
					err = fmt.Errorf("expected error associated to the target")
				} else if numTargets+1 > len(targets) {
					err = fmt.Errorf("more targets returned than injected")
				}
				if err != nil {
					routeResult <- err
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
