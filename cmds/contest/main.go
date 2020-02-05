// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/plugins/listeners/httplistener"
	"github.com/facebookincubator/contest/plugins/reporters/targetsuccess"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
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
	"github.com/facebookincubator/contest/plugins/teststeps/terminalexpect"
	"github.com/sirupsen/logrus"
)

const defaultDBURI = "contest:contest@tcp(localhost:3306)/contest?parseTime=true"

var (
	flagDBURI = flag.String("dbURI", defaultDBURI, "Database URI")
)

var targetManagers = map[string]target.TargetManagerFactory{
	csvtargetmanager.Name: csvtargetmanager.New,
	targetlist.Name:       targetlist.New,
}

var testFetchers = map[string]test.TestFetcherFactory{
	uri.Name:     uri.New,
	literal.Name: literal.New,
}

var testSteps = []test.TestStepLoader{
	echo.Load,
	slowecho.Load,
	example.Load,
	cmd.Load,
	sshcmd.Load,
	randecho.Load,
	terminalexpect.Load,
}

var reporters = map[string]job.ReporterFactory{
	targetsuccess.Name: targetsuccess.New,
}

// user-defined functions that will be made available to plugins for advanced
// expressions in config parameters.
var userFunctions = map[string]interface{}{
	// dummy function to prove that function registration works.
	"do_nothing": func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("do_nothing: no arg specified")
		}
		return a[0], nil
	},
}

func main() {
	flag.Parse()
	log := logging.GetLogger("contest")
	log.Level = logrus.DebugLevel

	pluginRegistry := pluginregistry.NewPluginRegistry()

	// Register TargetManager plugins
	for name, tmfactory := range targetManagers {
		if err := pluginRegistry.RegisterTargetManager(name, tmfactory); err != nil {
			log.Fatal(err)
		}
	}

	// Register TestFetcher plugins
	for name, tffactory := range testFetchers {
		if err := pluginRegistry.RegisterTestFetcher(name, tffactory); err != nil {
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
	for name, rfactory := range reporters {
		if err := pluginRegistry.RegisterReporter(name, rfactory); err != nil {
			log.Fatal(err)
		}
	}

	// storage initialization
	log.Infof("Using database URI: %s", *flagDBURI)
	storage.SetStorage(rdbms.New(*flagDBURI))

	// user-defined function registration
	for name, fn := range userFunctions {
		if err := test.RegisterFunction(name, fn); err != nil {
			log.Fatal(err)
		}
	}

	// spawn JobManager
	listener := httplistener.HTTPListener{}

	jm, err := jobmanager.New(&listener, pluginRegistry)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("JobManager %+v", jm)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	if err := jm.Start(sigs); err != nil {
		log.Fatal(err)
	}
}
