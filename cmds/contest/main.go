// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/facebookincubator/contest/pkg/abstract"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/plugins/listeners/httplistener"
	reportersNoop "github.com/facebookincubator/contest/plugins/reporters/noop"
	"github.com/facebookincubator/contest/plugins/reporters/targetsuccess"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"
	"github.com/facebookincubator/contest/plugins/targetlocker/mysql"
	targetLockerNoop "github.com/facebookincubator/contest/plugins/targetlocker/noop"
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

const (
	defaultDBURI        = "contest:contest@tcp(localhost:3306)/contest?parseTime=true"
	defaultTargetLocker = "MySQL:%dbURI%"
)

var (
	flagDBURI, flagTargetLocker *string
)

func setupFlags() {
	flagDBURI = flag.String("dbURI", defaultDBURI, "MySQL DSN")
	flagTargetLocker = flag.String("targetLocker", defaultTargetLocker,
		fmt.Sprintf("The engine to lock targets. Possible engines (the part before the first colon): %s",
			targetLockerFactories.ToAbstract(),
		))
	flag.Parse()
}

var log = logging.GetLogger("contest")

var targetManagers = []target.TargetManagerLoader{
	csvtargetmanager.Load,
	targetlist.Load,
}

var targetLockerFactories = target.LockerFactories{
	&mysql.Factory{},
	&inmemory.Factory{},
	&targetLockerNoop.Factory{},
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
	terminalexpect.Load,
}

var reporters = []job.ReporterLoader{
	targetsuccess.Load,
	reportersNoop.Load,
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

func expandArgument(arg string) string {
	// it does not support correct expanding into depth more one.
	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		arg = strings.Replace(arg, `%`+f.Name+`%`, f.Value.String(), -1)
	})
	return arg
}

func parseFactoryInfo(
	factories abstract.Factories,
	flagValue string,
) (factory abstract.Factory, factoryImplName, factoryArgument string) {
	factoryInfo := strings.SplitN(flagValue, `:`, 2)
	factoryImplName = factoryInfo[0]

	if len(factoryInfo) > 1 {
		factoryArgument = expandArgument(factoryInfo[1])
	}

	factory = factories.Find(factoryImplName)
	if factory == nil {
		log.Fatalf("Implementation '%s' is not found (possible values: %s)",
			factoryImplName, factories)
	}
	return
}

func main() {
	setupFlags()

	logrus.SetLevel(logrus.DebugLevel)
	log.Level = logrus.DebugLevel

	pluginRegistry := pluginregistry.NewPluginRegistry()

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

	// storage initialization
	log.Infof("Using database URI (MySQL DSN) for the main storage: %s", *flagDBURI)
	storage.SetStorage(rdbms.New(*flagDBURI))

	// set Locker engine
	targetLockerFactory, targetLockerImplName, targetLockerArgument :=
		parseFactoryInfo(targetLockerFactories.ToAbstract(), *flagTargetLocker)

	log.Infof("Using target locker '%s' with argument: '%s'", targetLockerImplName, targetLockerArgument)
	targetLocker, err := targetLockerFactory.(target.LockerFactory).New(config.LockTimeout, targetLockerArgument)
	if err != nil {
		log.Fatalf("Unable to initialize target locker: %v", err)
	}
	target.SetLocker(targetLocker)

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
