// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package plugins

import (
	"sync"

	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/userfunctions/donothing"
	"github.com/facebookincubator/contest/pkg/userfunctions/ocp"
	"github.com/facebookincubator/contest/pkg/xcontext"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"

	"github.com/facebookincubator/contest/plugins/reporters/noop"
	"github.com/facebookincubator/contest/plugins/reporters/targetsuccess"
	"github.com/facebookincubator/contest/plugins/targetmanagers/csvtargetmanager"
	"github.com/facebookincubator/contest/plugins/targetmanagers/targetlist"
	"github.com/facebookincubator/contest/plugins/testfetchers/literal"
	"github.com/facebookincubator/contest/plugins/testfetchers/uri"
	"github.com/facebookincubator/contest/plugins/teststeps/cmd"
	"github.com/facebookincubator/contest/plugins/teststeps/echo"
	"github.com/facebookincubator/contest/plugins/teststeps/example"
	"github.com/facebookincubator/contest/plugins/teststeps/randecho"
	"github.com/facebookincubator/contest/plugins/teststeps/sleep"
	"github.com/facebookincubator/contest/plugins/teststeps/sshcmd"
)

var TargetManagers = []target.TargetManagerLoader{
	csvtargetmanager.Load,
	targetlist.Load,
}

var testFetchers = []test.TestFetcherLoader{
	uri.Load,
	literal.Load,
}

var testSteps = []test.TestStepLoader{
	cmd.Load,
	echo.Load,
	example.Load,
	randecho.Load,
	sleep.Load,
	sshcmd.Load,
}

var reporters = []job.ReporterLoader{
	targetsuccess.Load,
	noop.Load,
}

var userFunctions = []map[string]interface{}{
	ocp.Load(),
	donothing.Load(),
}

var testInitOnce sync.Once

// Init initializes the plugin registry
func Init(pluginRegistry *pluginregistry.PluginRegistry, log xcontext.Logger) {

	// Register TargetManager plugins
	for _, tmloader := range TargetManagers {
		if err := pluginRegistry.RegisterTargetManager(tmloader()); err != nil {
			log.Fatalf("%v", err)
		}
	}

	// Register TestFetcher plugins
	for _, tfloader := range testFetchers {
		if err := pluginRegistry.RegisterTestFetcher(tfloader()); err != nil {
			log.Fatalf("%v", err)
		}
	}

	// Register TestStep plugins
	for _, tsloader := range testSteps {
		if err := pluginRegistry.RegisterTestStep(tsloader()); err != nil {
			log.Fatalf("%v", err)

		}
	}

	// Register Reporter plugins
	for _, rfloader := range reporters {
		if err := pluginRegistry.RegisterReporter(rfloader()); err != nil {
			log.Fatalf("%v", err)
		}
	}

	// user-defined function registration
	testInitOnce.Do(func() {
		for _, userFunction := range userFunctions {
			for name, fn := range userFunction {
				if err := test.RegisterFunction(name, fn); err != nil {
					log.Fatalf("%v", err)
				}
			}
		}
	})
}
