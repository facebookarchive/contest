// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package terminalexpect

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
	"github.com/insomniacslk/termhook"
)

// Name is the name used to look this plugin up.
var Name = "TerminalExpect"

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{}

// TerminalExpect reads from a terminal and returns when the given Match string
// is found on an output line.
type TerminalExpect struct {
	Port    string
	Speed   int
	Match   string
	Timeout time.Duration
}

// Name returns the plugin name.
func (ts TerminalExpect) Name() string {
	return Name
}

// match implements termhook.LineHandler
func match(match string, log xcontext.Logger) termhook.LineHandler {
	return func(w io.Writer, line []byte) (bool, error) {
		if strings.Contains(string(line), match) {
			log.Infof("%s: found pattern '%s'", Name, match)
			return true, nil
		}
		return false, nil
	}
}

// Run executes the terminal step.
func (ts *TerminalExpect) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	log := ctx.Logger()

	if err := ts.validateAndPopulate(params); err != nil {
		return err
	}
	hook, err := termhook.NewHook(ts.Port, ts.Speed, false, match(ts.Match, log))
	if err != nil {
		return err
	}
	// f implements plugins.PerTargetFunc
	f := func(ctx xcontext.Context, target *target.Target) error {
		errCh := make(chan error, 1)
		go func() {
			errCh <- hook.Run()
			if closeErr := hook.Close(); closeErr != nil {
				log.Errorf("Failed to close hook, err: %v", closeErr)
			}
		}()
		select {
		case err := <-errCh:
			return err
		case <-time.After(ts.Timeout):
			return fmt.Errorf("timed out after %s", ts.Timeout)
		case <-ctx.Done():
			return nil
		}
	}
	log.Debugf("%s: waiting for string '%s' with timeout %s", Name, ts.Match, ts.Timeout)
	return teststeps.ForEachTarget(Name, ctx, ch, f)
}

func (ts *TerminalExpect) validateAndPopulate(params test.TestStepParameters) error {
	// no expression expansion for these parameters
	port := params.GetOne("port")
	if port.IsEmpty() {
		return errors.New("invalid or missing 'port' parameter, must be exactly one string")
	}
	ts.Port = port.String()
	speed, err := params.GetInt("speed")
	if err != nil {
		return fmt.Errorf("invalid or missing 'speed' parameter: %v", err)
	}
	ts.Speed = int(speed)
	match := params.GetOne("match")
	if match.IsEmpty() {
		return errors.New("invalid or missing 'match' parameter, must be exactly one string")
	}
	ts.Match = match.String()
	timeoutStr := params.GetOne("timeout")
	if timeoutStr.IsEmpty() {
		return errors.New("invalid or missing 'timeout' parameter, must be exactly one string")
	}
	timeout, err := time.ParseDuration(timeoutStr.String())
	if err != nil {
		return fmt.Errorf("invalid terminal timeout %s", timeoutStr)
	}
	ts.Timeout = timeout
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *TerminalExpect) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	return ts.validateAndPopulate(params)
}

// New initializes and returns a new TerminalExpect test step.
func New() test.TestStep {
	return &TerminalExpect{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}
