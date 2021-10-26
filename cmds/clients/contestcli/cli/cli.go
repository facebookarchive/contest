// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/facebookincubator/contest/pkg/transport/http"

	flag "github.com/spf13/pflag"
)

const (
	defaultRequestor = "contestcli-http"
	jobWaitPoll      = 10 * time.Second
)

var (
	flagSet       *flag.FlagSet
	flagAddr      *string
	flagRequestor *string
	flagWait      *bool
	flagYAML      *bool
	flagStates    *[]string
	flagTags      *[]string
)

func initFlags(cmd string) {
	flagSet = flag.NewFlagSet(cmd, flag.ContinueOnError)
	flagAddr = flagSet.StringP("addr", "a", "http://localhost:8080", "ConTest server [scheme://]host:port[/basepath] to connect to")
	flagRequestor = flagSet.StringP("requestor", "r", defaultRequestor, "Identifier of the requestor of the API call")
	flagWait = flagSet.BoolP("wait", "w", false, "After starting a job, wait for it to finish, and exit 0 only if it is successful")
	flagYAML = flagSet.BoolP("yaml", "Y", false, "Parse job descriptor as YAML instead of JSON")

	// Flags for the "list" command.
	flagStates = flagSet.StringSlice("states", []string{}, "List of job states for the list command. A job must be in any of the specified states to match.")
	flagTags = flagSet.StringSlice("tags", []string{}, "List of tags for the list command. A job must have all the tags to match.")

	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(),
			`Usage:

  contestcli [flags] command

Commands:
  start [file]
        start a new job using the job description from the specified file
        or passed via stdin.
        when used with -wait flag, stdout will have two JSON outputs
        for job start and completion status separated with newline
  stop int
        stop a job by job ID
  status int
        get the status of a job by job ID
  retry int
        retry a job by job ID
  list [--states=JobStateStarted,...] [--tags=foo,...]
        list jobs by state and/or tags
  version
        request the API version to the server

Flags:
`)
		flagSet.PrintDefaults()
	}
}

func CLIMain(cmd string, args []string, stdout io.Writer) error {
	initFlags(cmd)
	if err := flagSet.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	return run(*flagRequestor, &http.HTTP{Addr: *flagAddr}, stdout)
}
