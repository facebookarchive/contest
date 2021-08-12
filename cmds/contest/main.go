// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// This is a generated file, edits should be made in the corresponding source
// file, and this file regenerated using
//   `contest-generator --from core-plugins.yml`
// followed by
//   `gofmt -w contest.go`

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/facebookincubator/contest/cmds/contest/server"

	// the targetmanager plugins
	csvtargetmanager "github.com/facebookincubator/contest/plugins/targetmanagers/csvtargetmanager"
	targetlist "github.com/facebookincubator/contest/plugins/targetmanagers/targetlist"

	// the testfetcher plugins
	literal "github.com/facebookincubator/contest/plugins/testfetchers/literal"
	uri "github.com/facebookincubator/contest/plugins/testfetchers/uri"

	// the teststep plugins
	ts_cmd "github.com/facebookincubator/contest/plugins/teststeps/cmd"
	sleep "github.com/facebookincubator/contest/plugins/teststeps/sleep"
	sshcmd "github.com/facebookincubator/contest/plugins/teststeps/sshcmd"

	// the reporter plugins
	targetsuccess "github.com/facebookincubator/contest/plugins/reporters/targetsuccess"
)

func getPluginConfig() *server.PluginConfig {
	var pc server.PluginConfig
	pc.TargetManagerLoaders = append(pc.TargetManagerLoaders, csvtargetmanager.Load)
	pc.TargetManagerLoaders = append(pc.TargetManagerLoaders, targetlist.Load)
	pc.TestFetcherLoaders = append(pc.TestFetcherLoaders, literal.Load)
	pc.TestFetcherLoaders = append(pc.TestFetcherLoaders, uri.Load)
	pc.TestStepLoaders = append(pc.TestStepLoaders, ts_cmd.Load)
	pc.TestStepLoaders = append(pc.TestStepLoaders, sleep.Load)
	pc.TestStepLoaders = append(pc.TestStepLoaders, sshcmd.Load)
	pc.ReporterLoaders = append(pc.ReporterLoaders, targetsuccess.Load)

	return &pc
}

func main() {
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	if err := server.Main(getPluginConfig(), os.Args[0], os.Args[1:], sigs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
