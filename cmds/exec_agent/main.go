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
)

var (
	flagSet *flag.FlagSet
)

func initFlags(cmd string) {
	flagSet = flag.NewFlagSet(cmd, flag.ContinueOnError)

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

	if err := run(); err != nil {
		log.Fatal(err)
	}
}
