// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package waitport provides an ability of waiting for a port on remote host to be opened for listening
package waitport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
const Name = "waitport"

// event names for this plugin.
const (
	EventCmdStart = event.Name(Name + "Start")
	EventCmdEnd   = event.Name(Name + "End")
)

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{
	EventCmdStart,
	EventCmdEnd,
}

// WaitPort provides an ability of waiting for a port on remote host to be opened for listening
type WaitPort struct {
}

// Name returns the plugin name.
func (ts *WaitPort) Name() string {
	return Name
}

// Run executes the cmd step.
func (ts *WaitPort) Run(ctx xcontext.Context, ch test.TestStepChannels, inputParams test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	params, err := parseParameters(inputParams)
	if err != nil {
		return nil, err
	}

	f := func(ctx xcontext.Context, targetWithData *teststeps.TargetWithData) error {
		target := targetWithData.Target
		targetParams, err := expandParameters(target, params)
		if err != nil {
			return err
		}

		// Emit EventCmdStart
		// Can emit duplicate events on server restart / job resumption
		payload, err := json.Marshal(targetParams)
		if err != nil {
			ctx.Warnf("Cannot encode payload for %T: %v", params, err)
		} else {
			rm := json.RawMessage(payload)
			evData := testevent.Data{
				EventName: EventCmdStart,
				Target:    target,
				Payload:   &rm,
			}
			if err := ev.Emit(ctx, evData); err != nil {
				ctx.Warnf("Cannot emit event EventCmdStart: %v", err)
			}
		}

		var resultAddresses []string
		portStr := strconv.Itoa(targetParams.Port)
		if len(targetParams.Address) > 0 {
			resultAddresses = append(resultAddresses, net.JoinHostPort(targetParams.Address, portStr))
		} else {
			if len(target.FQDN) > 0 {
				resultAddresses = append(resultAddresses, net.JoinHostPort(target.FQDN, portStr))
			}
			if len(target.PrimaryIPv4) > 0 {
				resultAddresses = append(resultAddresses, net.JoinHostPort(target.PrimaryIPv4.String(), portStr))
			}
			if len(target.PrimaryIPv6) > 0 {
				resultAddresses = append(resultAddresses, net.JoinHostPort(target.PrimaryIPv6.String(), portStr))
			}
		}

		// The timeout restarts after a server restart/resume
		finishedContext, cancel := context.WithTimeout(ctx, targetParams.Timeout)
		defer cancel()

		resultErr := func() error {
			for {
				for _, addr := range resultAddresses {
					var d net.Dialer
					conn, err := d.DialContext(finishedContext, targetParams.Protocol, addr)
					if err == nil {
						ctx.Warnf("successfully connected via %s", addr)
						if err := conn.Close(); err != nil {
							ctx.Warnf("failed to close opened connection: %v", err)
						}
						return nil
					}
					ctx.Warnf("failed to connect to '%s', err: '%v'", addr, err)
					if finishedContext.Err() != nil {
						return finishedContext.Err()
					}
				}

				ctx.Infof("wait for the next iteration")
				select {
				case <-finishedContext.Done():
					return finishedContext.Err()
				case <-time.After(targetParams.CheckInterval):
				}
			}
		}()

		// Emit EventCmdEnd
		evData := testevent.Data{
			EventName: EventCmdEnd,
			Target:    target,
			Payload:   nil,
		}
		if err := ev.Emit(ctx, evData); err != nil {
			ctx.Warnf("Cannot emit event EventCmdEnd: %v", err)
		}

		ctx.Infof("wait port plugin finished, err: '%v'", resultErr)
		return resultErr
	}
	return teststeps.ForEachTargetWithResume(ctx, ch, resumeState, 0, f)
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *WaitPort) ValidateParameters(ctx xcontext.Context, params test.TestStepParameters) error {
	_, err := parseParameters(params)
	return err
}

// New initializes and returns a new Cmd test step.
func New() test.TestStep {
	return &WaitPort{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}

var protocolOptions = []string{
	"tcp",
	"tcp4",
	"tcp6",
	"udp",
	"udp4",
	"udp6",
}

type parameters struct {
	target        *test.Param
	port          int
	protocol      string
	checkInterval time.Duration
	timeout       time.Duration
}

func parseParameters(params test.TestStepParameters) (*parameters, error) {
	const maxPortValue = 65535

	var target *test.Param
	targets := params.Get("target")
	switch len(targets) {
	case 0:
	case 1:
		target = &targets[0]
	default:
		return nil, errors.New("0 or 1 'target' parameters should be provided")
	}

	port, err := params.GetInt("port")
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'port' parameter: %w", err)
	}
	if port <= 0 || port > maxPortValue {
		return nil, fmt.Errorf("'port' parameter should be a positive integer that is less than %d, got: '%d'", maxPortValue, port)
	}

	protocols := params.Get("protocol")
	if len(protocols) != 1 {
		return nil, errors.New("a single 'protocol' should be provided")
	}
	protocol := strings.ToLower(protocols[0].String())

	isValidProtocol := func() bool {
		for _, opt := range protocolOptions {
			if opt == protocol {
				return true
			}
		}
		return false
	}()
	if !isValidProtocol {
		return nil, fmt.Errorf("'protocol' should be one of [%s]", strings.Join(protocolOptions, ", "))
	}

	timeout, err := parseDurationParam(params, "timeout")
	if err != nil {
		return nil, err
	}
	checkInterval, err := parseDurationParam(params, "check_interval")
	if err != nil {
		return nil, err
	}

	return &parameters{
		target:        target,
		port:          int(port),
		protocol:      protocol,
		checkInterval: checkInterval,
		timeout:       timeout,
	}, nil
}

type targetParameters struct {
	Address       string
	Port          int
	Protocol      string
	CheckInterval time.Duration
	Timeout       time.Duration
}

func expandParameters(t *target.Target, params *parameters) (*targetParameters, error) {
	var address string
	if params.target != nil {
		var err error
		address, err = params.target.Expand(t)
		if err != nil {
			return nil, fmt.Errorf("cannot expand target parameter '%s': '%v'", params.target.String(), err)
		}
	}

	return &targetParameters{
		Address:  address,
		Port:     params.port,
		Protocol: params.protocol,
		Timeout:  params.timeout,
	}, nil
}

func parseDurationParam(teststepParams test.TestStepParameters, parameterName string) (time.Duration, error) {
	var result time.Duration
	params := teststepParams.Get(parameterName)
	if len(params) != 1 {
		return result, fmt.Errorf("a single '%s' should be provided", parameterName)
	}
	var err error
	paramStr := params[0].String()
	result, err = time.ParseDuration(paramStr)
	if err != nil {
		return result, fmt.Errorf("failed to convert '%s' duration parameter, err: %v", paramStr, err)
	}
	return result, nil
}
