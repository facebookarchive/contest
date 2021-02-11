// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package plugins

import (
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/userfunctions/donothing"
	"github.com/facebookincubator/contest/pkg/userfunctions/ocp"

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
	"github.com/facebookincubator/contest/plugins/teststeps/slowecho"
	"github.com/facebookincubator/contest/plugins/teststeps/sshcmd"
	"github.com/sirupsen/logrus"
)

var targetManagers = []target.TargetManagerLoader{
	csvtargetmanager.Load,
	targetlist.Load,
}

var testFetchers = []test.TestFetcherLoader{
	uri.Load,
	literal.Load,
}

var testSteps = []test.TestStepLoader{
	echo.Load,
	slowecho.Load,
	example.Load,
	cmd.Load,
	sshcmd.Load,
	randecho.Load,
}

var reporters = []job.ReporterLoader{
	targetsuccess.Load,
	noop.Load,
}

var userFunctions = []map[string]interface{}{
	ocp.Load(),
	donothing.Load(),
}

// Init initializes the plugin registry
func Init(pluginRegistry *pluginregistry.PluginRegistry, log *logrus.Entry) {

	// Register TargetManager plugins
	for _, tmloader := range targetManagers {
		if err := pluginRegistry.RegisterTargetManager(tmloader()); err != nil {
			log.Fatal(err)
		}
	}

	// Register TestFetcher plugins
	for _, tfloader := range testFetchers {
		if err := pluginRegistry.RegisterTestFetcher(tfloader()); err != nil {
			log.Fatal(err)
		}
	}

	// Register TestStep plugins
	for _, tsloader := range testSteps {
		if err := pluginRegistry.RegisterTestStep(tsloader()); err != nil {
			log.Fatal(err)

		}
	}

	// Register Reporter plugins
	for _, rfloader := range reporters {
		if err := pluginRegistry.RegisterReporter(rfloader()); err != nil {
			log.Fatal(err)
		}
	}

	// user-defined function registration
	for _, userFunction := range userFunctions {
		for name, fn := range userFunction {
			if err := test.RegisterFunction(name, fn); err != nil {
				log.Fatal(err)
			}
		}
	}

}
