// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "Exec"

type Parameters struct {
	Bin struct {
		Path string   `json:"path"`
		Args []string `json:"args"`
	} `json:"bin"`
	OCPOutput bool `json:"ocp_output"`
}

// TestStep implementation for the exec plugin
type TestStep struct {
	params *Parameters
}

// Name returns the name of the Step
func (ts TestStep) Name() string {
	return Name
}

// Run executes the step.
func (ts *TestStep) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	f := func(ctx xcontext.Context, target *target.Target) error {
		ctx.Infof("Executing on target %s", target)

		cmd := exec.CommandContext(ctx, ts.params.Bin.Path, ts.params.Bin.Args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to get stdout pipe")
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start process: %w", err)
		}

		p := NewOCPEventParser(target, ev)
		dec := json.NewDecoder(stdout)
		for dec.More() {
			var root *OCPRoot
			dec.Decode(&root)

			p.Parse(ctx, root)
		}

		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("failed to wait on process: %w", err)
		}

		return p.Error()
	}
	return teststeps.ForEachTarget(Name, ctx, ch, f)
}

func canExecute(fi os.FileInfo) bool {
	// TODO: deal with acls?
	stat := fi.Sys().(*syscall.Stat_t)
	if stat.Uid == uint32(os.Getuid()) {
		return stat.Mode&0500 == 0500
	}

	if stat.Gid == uint32(os.Getgid()) {
		return stat.Mode&0050 == 0050
	}
	return stat.Mode&0005 == 0005

}

// ValidateParameters validates the parameters associated to the step
func (ts *TestStep) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	bag := params.GetOne("bag").JSON()

	if err := json.Unmarshal(bag, &ts.params); err != nil {
		return fmt.Errorf("no params")
	}

	// check binary exists and is executable
	fi, err := os.Stat(ts.params.Bin.Path)
	if err != nil {
		return fmt.Errorf("no such file")
	}

	if !fi.Mode().IsRegular() {
		return fmt.Errorf("not a file")
	}

	if !canExecute(fi) {
		return fmt.Errorf("file is not executable")
	}

	return nil
}

// New initializes and returns a new exec step.
func New() test.TestStep {
	return &TestStep{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}
