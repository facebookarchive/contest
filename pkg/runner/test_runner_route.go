// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/sirupsen/logrus"
)

// router implements the routing logic that injects targets into a test step and consumes
// targets in output from another test step
type stepRouter struct {
	log *logrus.Entry

	routingChannels routingCh
	bundle          test.TestStepBundle
	ev              testevent.EmitterFetcher

	timeouts TestRunnerTimeouts
}

func tryClose(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}

}

// routeIn is responsible for accepting a target from the previous routing block
// and injecting it into the associated test step. Returns the numer of targets
// injected into the test step or an error upon failure
func (r *stepRouter) routeIn(terminate <-chan struct{}) (int, error) {
	stepLabel := r.bundle.TestStepLabel
	log := logging.AddField(r.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "routeIn")

	log.Debugf("Initializing routeIn for %s", stepLabel)
	targetWriter := newTargetWriter(log, r.timeouts)

	// `ingressTarget` is used to keep track of ingress times of a target into a test step
	ingressTarget := make(map[string]time.Time)
	for {
		select {
		case <-terminate:
			return 0, fmt.Errorf("termination requested for routing into %s", stepLabel)
		case t, chanIsOpen := <-r.routingChannels.routeIn:
			if !chanIsOpen {
				close(r.routingChannels.stepIn)
				return len(ingressTarget), nil
			}
			log.Debugf("received target %v in input", t)
			ingressTarget[t.ID] = time.Now()
			err := targetWriter.writeTargetWithResult(terminate, t, r.routingChannels.stepIn)
			if err != nil {
				targetInErrEv := testevent.Data{EventName: target.EventTargetInErr, Target: t}
				if err := r.ev.Emit(targetInErrEv); err != nil {
					log.Warningf("could not emit %v event for target %+v: %v", targetInErrEv, *t, err)
				}
				return 0, fmt.Errorf("routing failed while injecting target %+v into %s", t, stepLabel)
			}
			targetInEv := testevent.Data{EventName: target.EventTargetIn, Target: t}
			if err := r.ev.Emit(targetInEv); err != nil {
				log.Warningf("could not emit %v event for Target: %+v", targetInEv, *t)
			}
		}
	}
}

func (r *stepRouter) emitOutEvent(t *target.Target, err error) error {

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
// and forward it to the next routing block. Returns the number of targets
// received from the test step or an error upon failure
func (r *stepRouter) routeOut(terminate <-chan struct{}) (int, error) {

	stepLabel := r.bundle.TestStepLabel
	log := logging.AddField(r.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "routeOut")

	targetWriter := newTargetWriter(log, r.timeouts)

	var err error

	log.Debugf("initializing routeOut for %s", stepLabel)
	// `egressTarget` is used to keep track of egress times of a target from a test step
	egressTarget := make(map[string]time.Time)

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

			if _, targetPresent := egressTarget[t.ID]; targetPresent {
				err = fmt.Errorf("step %s returned target %+v multiple times", r.bundle.TestStepLabel, t)
				break
			}
			// Emit an event signaling that the target has left the TestStep
			if err := r.emitOutEvent(t, nil); err != nil {
				log.Warningf("could not emit out event for target %v: %v", *t, err)
			}
			// Register egress time and forward target to the next routing block
			egressTarget[t.ID] = time.Now()
			if err := targetWriter.writeTimeout(terminate, r.routingChannels.routeOut, t, r.timeouts.MessageTimeout); err != nil {
				log.Panicf("could not forward target to the test runner: %+v", err)
			}
		case targetError, chanIsOpen := <-r.routingChannels.stepErr:
			if !chanIsOpen {
				log.Debugf("step error closed")
				r.routingChannels.stepErr = nil
				break
			}

			if _, targetPresent := egressTarget[targetError.Target.ID]; targetPresent {
				err = fmt.Errorf("step %s returned target %+v multiple times", r.bundle.TestStepLabel, targetError.Target)
			} else {
				if err := r.emitOutEvent(targetError.Target, targetError.Err); err != nil {
					log.Warningf("could not emit err event for target: %v", *targetError.Target)
				}
				egressTarget[targetError.Target.ID] = time.Now()
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
		log.Debugf("routeOut failed: %v", err)
		return 0, err
	}
	return len(egressTarget), nil

}

// route implements the routing logic from the previous routing block to the test step
// and from the test step to the next routing block
func (r *stepRouter) route(terminate <-chan struct{}, resultCh chan<- routeResult) {

	var (
		inTargets, outTargets               int
		routingErr                          error
		routeInCompleted, routeOutCompleted bool
	)
	terminateInternal := make(chan struct{})
	errRouteInCh := make(chan error)
	errRouteOutCh := make(chan error)

	go func() {
		var err error
		inTargets, err = r.routeIn(terminateInternal)
		errRouteInCh <- err
	}()

	go func() {
		var err error
		outTargets, err = r.routeOut(terminateInternal)
		errRouteOutCh <- err
	}()

	for {
		select {
		case err := <-errRouteInCh:
			routeInCompleted = true

			if err != nil {
				tryClose(terminateInternal)
			}
			if routingErr == nil {
				routingErr = err
			}
		case err := <-errRouteOutCh:
			routeOutCompleted = true
			if err != nil {
				tryClose(terminateInternal)
			}
			if routingErr == nil {
				routingErr = err
			}
		case <-terminate:
			terminate = nil
			tryClose(terminateInternal)
		}
		if routeInCompleted && routeOutCompleted {
			break
		}
	}

	if routingErr == nil && inTargets != outTargets {
		routingErr = fmt.Errorf("step %s completed but did not return all injected Targets (%d!=%d)", r.bundle.TestStepLabel, inTargets, outTargets)
	}

	// Send the result to the test runner, which is expected to be listening
	// within `MessageTimeout`. If that's not the case, we hit an unrecovrable
	// condition.
	select {
	case resultCh <- routeResult{bundle: r.bundle, err: routingErr}:
	case <-time.After(r.timeouts.MessageTimeout):
		log.Panicf("could not send routing block result")
	}
}

func newStepRouter(log *logrus.Entry, bundle test.TestStepBundle, routingChannels routingCh, ev testevent.EmitterFetcher, timeouts TestRunnerTimeouts) *stepRouter {
	routerLogger := logging.AddField(log, "step", bundle.TestStepLabel)
	r := stepRouter{log: routerLogger, bundle: bundle, routingChannels: routingChannels, ev: ev, timeouts: timeouts}
	return &r
}
