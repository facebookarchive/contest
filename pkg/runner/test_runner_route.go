// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"container/list"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/sirupsen/logrus"
)

// router implements the routing logic that injects targets into a test step and consumes
// targets in output from another test step
type router struct {
	log *logrus.Entry

	routingChannels routingCh
	bundle          test.StepBundle
	ev              testevent.EmitterFetcher

	timeouts TestRunnerTimeouts
}

// routeIn is responsible for accepting a target from the previous routing block
// and injecting it into the associated test step. Returns the numer of targets
// injected into the test step or an error upon failure
func (r *router) routeIn(terminate <-chan struct{}) (int, error) {

	stepLabel := r.bundle.StepLabel
	log := logging.AddField(r.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "routeIn")

	var (
		pendingTarget *target.Target
		injectionWg   sync.WaitGroup
		err           error
	)

	// terminateInternal is a control channel used to propagate termination
	// signal to goroutines internal to routeIn
	terminateInternal := make(chan struct{})

	// targets buffers targets coming from the previous routing blocks,
	// queueing them for injection into the Step. The list is accessed
	// synchronously by a single goroutine.
	targets := list.New()

	// ingressTarget is used to keep track of ingress times of a target into a test step
	ingressTarget := make(map[*target.Target]time.Time)

	// targetResultCh receives results from the goroutine which writes targets into a test step
	targetResultCh := make(chan *target.Result)

	log.Debugf("initializing routeIn for %s", stepLabel)
	targetWriter := newTargetWriter(log, r.timeouts)

	for {
		select {
		case <-terminate:
			err = fmt.Errorf("termination requested for routing into %s", stepLabel)
		case targetResult := <-targetResultCh:
			t := targetResult.Target
			log.Debugf("received injection result for %v", t)
			pendingTarget = nil
			if targetResult.Err != nil {
				err = fmt.Errorf("routing failed while injecting target %+v into %s", targetResult.Err, stepLabel)
				targetInErrEv := testevent.Data{EventName: target.EventTargetInErr, Target: t}
				if err := r.ev.Emit(targetInErrEv); err != nil {
					log.Warningf("could not emit %v event for target: %+v", targetInErrEv, *t)
				}
			} else {
				targetInEv := testevent.Data{EventName: target.EventTargetIn, Target: t}
				if err := r.ev.Emit(targetInEv); err != nil {
					log.Warningf("could not emit %v event for Target: %+v", targetInEv, *t)
				}
			}
		case t, chanIsOpen := <-r.routingChannels.routeIn:
			if !chanIsOpen {
				log.Debugf("routing input channel closed")
				r.routingChannels.routeIn = nil
			} else {
				log.Debugf("received target %v in input", t)
				targets.PushFront(t)
			}
		}

		if err != nil {
			break
		}
		if targets.Len() > 0 && pendingTarget == nil {
			// at least one more target available to inject to the test step
			pendingTarget = targets.Back().Value.(*target.Target)
			ingressTarget[pendingTarget] = time.Now()
			targets.Remove(targets.Back())
			injectionWg.Add(1)
			log.Debugf("writing target %v into test step", pendingTarget)
			go func() {
				defer injectionWg.Done()
				err := targetWriter.writeTarget(terminateInternal, r.routingChannels.stepIn, pendingTarget, r.timeouts.MessageTimeout)
				select {
				case <-terminateInternal:
				case targetResultCh <- &target.Result{Target: pendingTarget, Err: err}:
				case <-time.After(r.timeouts.MessageTimeout):
					log.Panicf("timeout while writing result for target %+v after %v", pendingTarget, r.timeouts.MessageTimeout)
				}
			}()

		}
		if targets.Len() == 0 && pendingTarget == nil && r.routingChannels.routeIn == nil {
			// no more targets in the buffer not pending, and input channel is closed
			// routing logic should return.
			log.Debugf("input channel is closed and no more targets are available, closing step input channel")
			close(r.routingChannels.stepIn)
			break
		}
	}
	// Signal termination to the injection routines regardless of the result of the
	// routing. If the routing completed successfully, this is a no-op. If there is an
	// injection goroutine running, wait for it to terminate, as we might have gotten
	// here after a cancellation signal.
	close(terminateInternal)
	injectionWg.Wait()

	r.log.Debugf("route in terminating")
	if err != nil {
		return 0, err
	}
	return len(ingressTarget), nil
}

func (r *router) emitOutEvent(t *target.Target, err error) error {

	log := logging.AddField(r.log, "step", r.bundle.StepLabel)
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
// injected into the test step or an error upon failure
func (r *router) routeOut(terminate <-chan struct{}) (int, error) {

	stepLabel := r.bundle.StepLabel
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
			err = fmt.Errorf("termination requested for routing into %s", r.bundle.StepLabel)
		case targetResult, chanIsOpen := <-r.routingChannels.stepOut:
			if !chanIsOpen {
				log.Debugf("step output closed")
				r.routingChannels.stepOut = nil
				break
			}

			t := targetResult.Target
			if _, targetPresent := egressTarget[t]; targetPresent {
				err = fmt.Errorf("step %s returned target %+v multiple times", r.bundle.StepLabel, t)
				break
			}

			if targetResult.Err != nil {
				if err := r.emitOutEvent(t, targetResult.Err); err != nil {
					log.Warningf("could not emit err event for target: %v", *t)
				}
				egressTarget[t] = time.Now()
				select {
				case <-terminate:
					err = fmt.Errorf("termination requested for routing into %s", r.bundle.StepLabel)
				case r.routingChannels.targetResult <- targetResult:
				case <-time.After(r.timeouts.MessageTimeout):
					log.Panicf("could not forward failed target (%+v) to the test runner: %v", t, targetResult.Err)
				}

			} else {
				// Emit an event signaling that the target has left the Step
				if err := r.emitOutEvent(t, nil); err != nil {
					log.Warningf("could not emit out event for target: %v", *t)
				}
				// Register egress time and forward target to the next routing block
				egressTarget[t] = time.Now()
				if err := targetWriter.writeTarget(terminate, r.routingChannels.routeOut, t, r.timeouts.MessageTimeout); err != nil {
					log.Panicf("could not forward successful target to the the next routing block: %+v", err)
				}
			}
		}
		if err != nil {
			break
		}
		if r.routingChannels.stepOut == nil {
			log.Debugf("output and error channel from step are closed, routeOut should terminate")
			close(r.routingChannels.routeOut)
			break
		}
	}

	r.log.Debugf("route out terminating")
	if err != nil {
		return 0, err
	}
	return len(egressTarget), nil

}

// route implements the routing logic from the previous routing block to the test step
// and from the test step to the next routing bloxk
func (r *router) route(terminate <-chan struct{}, resultCh chan<- routeResult) {

	var (
		inTargets, outTargets int
		inErr, outErr, err    error
		routeWg               sync.WaitGroup
	)

	r.log.Debugf("initializing routing")

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
		err = fmt.Errorf("step %s completed but did not return all injected Targets (%d!=%d)", r.bundle.StepLabel, inTargets, outTargets)
	}

	r.log.Debugf("sending back route result")
	// Send the result to the test runner, which is expected to be listening
	// within `MessageTimeout`. If that's not the case, we hit an unrecovrable
	// condition.
	select {
	case resultCh <- routeResult{bundle: r.bundle, err: err}:
	case <-time.After(r.timeouts.MessageTimeout):
		r.log.Panicf("could not send routing block result")
	}
}

func newRouter(log *logrus.Entry, bundle test.StepBundle, routingChannels routingCh, ev testevent.EmitterFetcher, timeouts TestRunnerTimeouts) *router {
	r := router{log: log, bundle: bundle, routingChannels: routingChannels, ev: ev, timeouts: timeouts}
	return &r
}
