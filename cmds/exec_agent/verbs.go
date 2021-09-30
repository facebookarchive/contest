// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func run() error {
	verb := strings.ToLower(flagSet.Arg(0))
	if verb == "" {
		return fmt.Errorf("missing verb, see --help")
	}

	switch verb {
	case "start":
		bin := flagSet.Arg(1)
		if bin == "" {
			return fmt.Errorf("missing binary argument, see --help")
		}

		var args []string
		if flagSet.NArg() > 2 {
			args = flagSet.Args()[2:]
		}
		return start(bin, args)

	default:
		return fmt.Errorf("invalid verb: %s", verb)
	}
}

func start(bin string, args []string) error {
	cmd := exec.Command(bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	log.Printf("out: %s", stdout.String())

	return runErr
}
