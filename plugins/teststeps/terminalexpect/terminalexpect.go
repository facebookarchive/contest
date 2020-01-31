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

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/plugins/teststeps"
	"github.com/insomniacslk/termhook"
)

// Name is the name used to look this plugin up.
var Name = "TerminalExpect"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

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
func match(match string) termhook.LineHandler {
	return func(w io.Writer, line []byte) (bool, error) {
		if strings.Contains(string(line), match) {
			log.Infof("%s: found pattern '%s'", Name, match)
			return true, nil
		}
		return false, nil
	}
}

// Run executes the terminal step.
func (ts *TerminalExpect) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	if err := ts.validateAndPopulate(params); err != nil {
		return err
	}
	hook, err := termhook.NewHook(ts.Port, ts.Speed, match(ts.Match))
	if err != nil {
		return err
	}
	hook.ReadOnly = true
	// f implements plugins.PerTargetFunc
	f := func(cancel, pause <-chan struct{}, target *target.Target) error {
		errCh := make(chan error)
		go func() {
			errCh <- hook.Run()
		}()
		select {
		case err := <-errCh:
			return err
		case <-time.After(ts.Timeout):
			return fmt.Errorf("timed out after %s", ts.Timeout)
		case <-cancel:
			return nil
		case <-pause:
			return nil
		}
	}
	log.Printf("%s: waiting for string '%s' with timeout %s", Name, ts.Match, ts.Timeout)
	return teststeps.ForEachTarget(Name, cancel, pause, ch, f)
}

func (ts *TerminalExpect) validateAndPopulate(params test.TestStepParameters) error {
	// no expression expansion for these parameters
	port := params.GetOne("port")
	if port.IsEmpty() {
		return errors.New("invalid or missing 'port' parameter, must be exactly one string")
	}
	ts.Port = port.Raw()
	speed, err := params.GetInt("speed")
	if err != nil {
		return fmt.Errorf("invalid or missing 'speed' parameter: %v", err)
	}
	ts.Speed = int(speed)
	match := params.GetOne("match")
	if match.IsEmpty() {
		return errors.New("invalid or missing 'match' parameter, must be exactly one string")
	}
	ts.Match = match.Raw()
	timeoutStr := params.GetOne("timeout")
	if timeoutStr.IsEmpty() {
		return errors.New("invalid or missing 'timeout' parameter, must be exactly one string")
	}
	timeout, err := time.ParseDuration(timeoutStr.Raw())
	if err != nil {
		return fmt.Errorf("invalid terminal timeout %s", timeoutStr)
	}
	ts.Timeout = timeout
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *TerminalExpect) ValidateParameters(params test.TestStepParameters) error {
	return ts.validateAndPopulate(params)
}

// Resume tries to resume a previously interrupted test step. TerminalExpect cannot
// resume.
func (ts *TerminalExpect) Resume(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *TerminalExpect) CanResume() bool {
	return false
}

// New initializes and returns a new TerminalExpect test step.
func New() test.TestStep {
	return &TerminalExpect{}
}
