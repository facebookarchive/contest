// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "Cmd"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

// event names for this plugin.
const (
	EventCmdStart = event.Name("CmdStart")
	EventCmdEnd   = event.Name("CmdEnd")
)

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{
	EventCmdStart,
	EventCmdEnd,
}

// eventCmdStartPayload is the payload of an EventStartCmd event, and it
// contains the expanded command matching the Cmd struct.
type eventCmdStartPayload struct {
	Path string
	Args []string
	Dir  string
}

// Cmd is used to run arbitrary commands as test steps.
type Cmd struct {
	executable string
	args       []test.Param
	dir        string
}

// Name returns the plugin name.
func (ts Cmd) Name() string {
	return Name
}

// Run executes the cmd step.
func (ts *Cmd) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	if err := ts.validateAndPopulate(params); err != nil {
		return err
	}
	f := func(cancel, pause <-chan struct{}, target *target.Target) error {
		ctx, ctxCancel := context.WithCancel(context.Background())
		defer ctxCancel()
		// expand args
		var args []string
		for _, arg := range ts.args {
			expArg, err := arg.Expand(target)
			if err != nil {
				return fmt.Errorf("failed to expand argument '%s': %v", arg, err)
			}
			args = append(args, expArg)
		}
		cmd := exec.CommandContext(ctx, ts.executable, args...)
		cmd.Dir = ts.dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if ts.dir != "" {
			log.Printf("Running command '%+v' in directory '%+v'", cmd, cmd.Dir)
		} else {
			log.Printf("Running command '%+v'", cmd)
		}

		errCh := make(chan error)
		go func() {
			// Emit EventCmdStart
			payload, err := json.Marshal(eventCmdStartPayload{Path: cmd.Path, Args: cmd.Args, Dir: cmd.Dir})
			if err != nil {
				log.Warningf("Cannot encode payload for EventCmdStart: %v", err)
			} else {
				rm := json.RawMessage(payload)
				evData := testevent.Data{
					EventName: EventCmdStart,
					Target:    target,
					Payload:   &rm,
				}
				if err := ev.Emit(evData); err != nil {
					log.Warningf("Cannot emit event EventCmdStart: %v", err)
				}
			}
			// Run the command
			errCh <- cmd.Run()
			// Emit EventCmdEnd
			evData := testevent.Data{
				EventName: EventCmdStart,
				Target:    target,
				Payload:   nil,
			}
			if err := ev.Emit(evData); err != nil {
				log.Warningf("Cannot emit event EventCmdEnd: %v", err)
			}
			log.Infof("Stdout of command '%s' with args '%s' is '%s'", cmd.Path, cmd.Args, stdout.Bytes())
		}()
		select {
		case err := <-errCh:
			log.Warningf("Stderr of command '%+v' is: '%s'", cmd, stderr.Bytes())
			return err
		case <-cancel:
			return nil
		case <-pause:
			return nil
		}
	}
	return teststeps.ForEachTarget(Name, cancel, pause, ch, f)
}

func (ts *Cmd) validateAndPopulate(params test.TestStepParameters) error {
	param := params.GetOne("executable")
	if param.IsEmpty() {
		return errors.New("invalid or missing 'executable' parameter, must be exactly one string")
	}
	ex := param.String()
	if filepath.IsAbs(ex) {
		ts.executable = ex
	} else {
		p, err := exec.LookPath(ex)
		if err != nil {
			return fmt.Errorf("cannot find '%s' executable in PATH: %v", ex, err)
		}
		// the call could still fail later if the file is removed, is not
		// executable, etc, but at least we do basic checks here.
		ts.executable = p
	}
	ts.args = params.Get("args")
	dir := params.GetOne("dir")
	ts.dir = dir.String()
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *Cmd) ValidateParameters(params test.TestStepParameters) error {
	return ts.validateAndPopulate(params)
}

// Resume tries to resume a previously interrupted test step. Cmd cannot
// resume.
func (ts *Cmd) Resume(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *Cmd) CanResume() bool {
	return false
}

// New initializes and returns a new Cmd test step.
func New() test.TestStep {
	return &Cmd{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}
