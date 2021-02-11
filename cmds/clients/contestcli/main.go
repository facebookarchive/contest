// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/facebookincubator/contest/pkg/transport/http"

	flag "github.com/spf13/pflag"
)

// Unauthenticated, unencrypted sample HTTP client for ConTest.
// Requires the `httplistener` plugin for the API listener.
//
// Usage examples:
// Start a job with the provided job description from a JSON file
//   ./contestcli-http start < start.json
//
// Get the status of a job whose ID is 10
//   ./contestcli-http status 10

const (
	defaultRequestor = "contestcli-http"
	jobWaitPoll      = 10 * time.Second
)

var (
	flagAddr      = flag.StringP("addr", "a", "http://localhost:8080", "ConTest server [scheme://]host:port[/basepath] to connect to")
	flagRequestor = flag.StringP("requestor", "r", defaultRequestor, "Identifier of the requestor of the API call")
	flagWait      = flag.BoolP("wait", "w", false, "After starting a job, wait for it to finish, and exit 0 only if it is successful")
	flagYAML      = flag.BoolP("yaml", "Y", false, "Parse job descriptor as YAML instead of JSON")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of contestcli-http:\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  contestcli-http [args] command\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "command: start, stop, status, retry, version\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  start\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        start a new job using the job description passed via stdin\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        when used with -wait flag, stdout will have two JSON outputs\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        for job start and completion status separated with newline\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  stop int\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        stop a job by job ID\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  status int\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        get the status of a job by job ID\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  retry int\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        retry a job by job ID\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        request the API version to the server\n")
		fmt.Fprintf(flag.CommandLine.Output(), "\nargs:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if err := run(*flagRequestor, &http.HTTP{Addr: *flagAddr}); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
