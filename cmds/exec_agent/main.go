// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"syscall"
)

var (
	flagSet   *flag.FlagSet
	flagDebug *bool
)

func initFlags(cmd string) {
	flagSet = flag.NewFlagSet(cmd, flag.ContinueOnError)
	flagDebug = flagSet.Bool("debug", false, "Output logs and errors in foreground, otherwise close stderr")

	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(),
			`Usage:

  %s [flags] command

Commands:
  start /path/to/binary <args>
        start a new binary and detach from the controlling TTY if any

Flags:
`, path.Base(cmd))
		flagSet.PrintDefaults()
	}
}

func main() {
	initFlags(os.Args[0])

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		log.Fatalf("failed to parse args: %v", err)
	}

	// when not run as debug, redirect stdin and stderr to /dev/null
	if !*flagDebug {
		null, err := os.Open(os.DevNull)
		if err != nil {
			log.Fatal(err)
		}
		defer null.Close()

		syscall.Dup2(int(null.Fd()), 0)
		syscall.Dup2(int(null.Fd()), 2)
	}

	if err := run(); err != nil {
		log.Fatal(err)
	}
}
