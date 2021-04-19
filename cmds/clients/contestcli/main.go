// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"fmt"
	"os"

	"github.com/facebookincubator/contest/cmds/clients/contestcli/cli"
)

// Unauthenticated, unencrypted sample HTTP client for ConTest.
// Requires the `httplistener` plugin for the API listener.
//
// Usage examples:
// Start a job with the provided job description from a JSON file
//   ./contestcli start start.json
//
// Get the status of a job whose ID is 10
//   ./contestcli status 10
//
// List all the jobs:
//   ./contestcli list
//
// List all the failed jobs:
//   ./contestcli list -state JobStateFailed
//
// List all the failed jobs with tags "foo" and "bar":
//   ./contestcli list -state JobStateFailed -tags foo,bar

func main() {
	if err := cli.CLIMain(os.Args[0], os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
