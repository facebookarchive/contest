// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
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

	ingressMap map[*target.Target]time.Time
	egressMap  map[*target.Target]time.Time
	mapLock    *sync.Mutex

	routeInDone chan struct{}
}

func (r *router) forwardTarget(terminate, terminateInternal <-chan struct{}, targetResult *target.Result) {
	if targetResult.Err != nil {
		if err := r.emitOutEvent(targetResult.Target, targetResult.Err); err != nil {
			r.log.Warningf("could not emit err event for target: %v", *targetResult)
		}
		// double terminate should be really replaced by context
		select {
		case <-terminate:
		case <-terminateInternal:
		case r.routingChannels.targetErr <- targetResult:
		}
	} else {
		if err := r.emitOutEvent(targetResult.Target, nil); err != nil {
			r.log.Warningf("could not emit out event for target: %v", *targetResult)
		}
		select {
		// double terminate should be really replaced by context
		case <-terminate:
		case <-terminateInternal:
		case r.routingChannels.routeOut <- targetResult.Target:
		}
	}
}

// routeIn is responsible for accepting a target from the previous routing block
// and injecting it into the associated test step. Returns the numer of targets
// injected into the test step or an error upon failure
func (r *router) routeIn(terminate <-chan struct{}) error {

	stepLabel := r.bundle.StepLabel
	log := logging.AddField(r.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "routeIn")

	var err error
routing:
	for {
		select {
		case <-terminate:
			err = fmt.Errorf("termination requested for routing into %s", stepLabel)
		case t, chanIsOpen := <-r.routingChannels.routeIn:
			if !chanIsOpen {
				log.Debugf("routing input channel closed")
				r.routingChannels.routeIn = nil
				close(r.routingChannels.stepIn)
				break routing
			}
			select {
			case <-terminate:
				err = fmt.Errorf("termination requested for routing into %s", stepLabel)
			case r.routingChannels.stepIn <- t:
				log.Debugf("target %v injected into %s", t, stepLabel)
				r.mapLock.Lock()
				r.ingressMap[t] = time.Now()
				r.mapLock.Unlock()
				targetInEv := testevent.Data{EventName: target.EventTargetIn, Target: t}
				if err := r.ev.Emit(targetInEv); err != nil {
					log.Warningf("could not emit %v event for Target: %+v", targetInEv, *t)
				}
			case <-time.After(r.timeouts.MessageTimeout):
				err = fmt.Errorf("timeout while writing target %+v", t)
				targetInErrEv := testevent.Data{EventName: target.EventTargetInErr, Target: t}
				if err := r.ev.Emit(targetInErrEv); err != nil {
					log.Warningf("could not emit %v event for target: %+v", targetInErrEv, *t)
				}
			}
		}
		if err != nil {
			break
		}
	}
	log.Debugf("route in terminating with err: %v", err)
	if err != nil {
		return err
	}
	close(r.routeInDone)

	return nil
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
func (r *router) routeOut(terminate <-chan struct{}) error {

	stepLabel := r.bundle.StepLabel
	log := logging.AddField(r.log, "step", stepLabel)
	log = logging.AddField(log, "phase", "routeOut")

	var (
		err               error
		wgEgress          sync.WaitGroup
		terminateAsserted = false
	)

	log.Debugf("initializing routeOut for %s", stepLabel)
	terminateInternal := make(chan struct{})

	for {
		select {
		case <-terminate:
			close(terminateInternal)
			terminateAsserted = true
			err = fmt.Errorf("termination requested for routing into %s", r.bundle.StepLabel)
		case <-r.routeInDone:
			r.routeInDone = nil
		case targetResult, chanIsOpen := <-r.routingChannels.stepOut:
			if !chanIsOpen {
				log.Debugf("step output closed")
				r.routingChannels.stepOut = nil
				break
			}
			log.Debugf("received on stepOut: %v", targetResult)
			r.mapLock.Lock()
			t := targetResult.Target
			if _, targetPresent := r.egressMap[t]; targetPresent {
				err = fmt.Errorf("step %s returned target %+v multiple times", r.bundle.StepLabel, t)
				r.mapLock.Unlock()
				break
			}
			r.egressMap[t] = time.Now()
			r.mapLock.Unlock()
			wgEgress.Add(1)
			go func(t *target.Result) {
				defer wgEgress.Done()
				r.forwardTarget(terminate, terminateInternal, t)
			}(targetResult)
		}
		if err != nil {
			if !terminateAsserted {
				close(terminateInternal)
			}
			break
		}

		r.mapLock.Lock()
		targetsBalanced := len(r.ingressMap) == len(r.egressMap)
		r.mapLock.Unlock()

		if r.routeInDone == nil && targetsBalanced {
			// no more targets are expected to come in. If the step is bugged
			// and more targets will be ejected, nobody will be listening, so
			// it will most likely block and be caught by timeout.
			break
		}
		if r.routingChannels.stepOut == nil {
			if !targetsBalanced {
				err = fmt.Errorf("not all targets have been returned from step")
			}
			log.Debugf("output and error channel from step are closed, will wait for injection to terminate")
			break
		}
	}

	// next routing block guarantees that target shall be injected with a timeout, so in the worst case
	// injection goroutines will receive a termination signal
	wgEgress.Wait()
	close(r.routingChannels.routeOut)
	r.log.Debugf("route out terminating")
	if err != nil {
		return err
	}
	return nil

}

// route implements the routing logic from the previous routing block to the test step
// and from the test step to the next routing bloxk
func (r *router) route(terminate <-chan struct{}, pause <-chan struct{}, resultCh chan<- routeResult) {

	var (
		inErr, outErr, err   error
		cancellationAsserted bool
	)

	routeInCh := make(chan error)
	routeOutCh := make(chan error)

	terminateInternal := make(chan struct{})
	r.log.Debugf("initializing routing")

	go func() {
		routeInCh <- r.routeIn(terminateInternal)
	}()
	go func() {
		routeOutCh <- r.routeOut(terminateInternal)
	}()

	for {
		select {
		case <-terminate:
			cancellationAsserted = true
			close(terminateInternal)
		case <-pause:
			cancellationAsserted = true
			close(terminateInternal)
		case outErr = <-routeOutCh:
			routeOutCh = nil
		case inErr = <-routeInCh:
			routeInCh = nil
		}
		if inErr != nil || outErr != nil {
			cancellationAsserted = true
			close(terminateInternal)
		}
		if cancellationAsserted {
			if routeInCh != nil {
				<-routeInCh
			}
			if routeOutCh != nil {
				<-routeOutCh
			}
			break
		}
		if routeInCh == nil && routeOutCh == nil {
			break
		}
	}

	if inErr != nil {
		err = inErr
	}
	if outErr != nil {
		err = outErr
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
	r.ingressMap = make(map[*target.Target]time.Time)
	r.egressMap = make(map[*target.Target]time.Time)
	r.mapLock = &sync.Mutex{}
	r.routeInDone = make(chan struct{})

	return &r
}
