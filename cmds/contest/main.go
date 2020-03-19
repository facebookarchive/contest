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

// targetLockerNames returns a slice of registered implementation names of target.Locker
func targetLockerNames() (result []string) {
	for _, targetLocker := range targetLockers {
		name, _ := targetLocker()
		result = append(result, name)
	}
	return
}

// setupFlags handles everything related to package `flag`.
func setupFlags() {
	flag.Usage = func() {
		flag.PrintDefaults()
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "\nOptions supports macroses. "+
			"For example, to insert the value of flag 'dbURI' anywhere just use macros '%%dbURI%%'. "+
			"But recursive macroses are not supported.\n")
	}

	flagDBURI = flag.String("dbURI", defaultDBURI, "MySQL DSN")

	flagTargetLocker = flag.String("targetLocker", defaultTargetLocker,
		fmt.Sprintf("The engine to lock targets. Possible engines (the part before the first colon): %s. "+
			"After the colon a DSN (where to store the locks) is expected. "+
			"An example of the total value: 'MySQL:myuser:mypass@tcp(localhost:3306)/mydb?parseTime=true'. See also https://github.com/Go-SQL-Driver/MySQL/#dsn-data-source-name",
			strings.Join(targetLockerNames(), ", "),
		))
	flag.Parse()
}

var log = logging.GetLogger("contest")
var pluginRegistry *pluginregistry.PluginRegistry

var targetManagers = []target.TargetManagerLoader{
	csvtargetmanager.Load,
	targetlist.Load,
}

var targetLockers = []target.LockerLoader{
	mysql.Load,
	inmemory.Load,
	targetLockerNoop.Load,
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

// expandArgument expands macroses like '%dbURI%' to values of flags with such names.
func expandArgument(arg string) string {
	// it does not support recursive expanding
	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		arg = strings.Replace(arg, `%`+f.Name+`%`, f.Value.String(), -1)
	})
	return arg
}

// newTargetLockerFromFlag parses flag `-targetLocker` and constructs the requested
// target.Locker. If an error occurs then the execution of the program will
// be terminated (via `log.Fatal*`).
//
// Returned values are the target.Locker, the name of the implementation, and
// the "storageDSN" (see `setupFlags`, `target.LockerFactory`).
func newTargetLockerFromFlag(
	flagValue string,
) (targetLocker target.Locker, implName string, storageDSN string) {
	targetLockerInfo := strings.SplitN(flagValue, `:`, 2)
	implName = targetLockerInfo[0]

	if len(targetLockerInfo) > 1 {
		storageDSN = expandArgument(targetLockerInfo[1])
	}

	var err error
	targetLocker, err = pluginRegistry.NewTargetLocker(implName, config.LockTimeout, storageDSN)
	if err != nil {
		log.Fatalf("unable to initialize target locker '%s' (with DSN: '%s'): %v", implName, storageDSN, err)
	}
	return targetLocker, implName, storageDSN
}

func main() {
	setupFlags()

	logrus.SetLevel(logrus.DebugLevel)
	log.Level = logrus.DebugLevel

	pluginRegistry = pluginregistry.NewPluginRegistry()

	// Register TargetManager plugins
	for _, tmloader := range targetManagers {
		if err := pluginRegistry.RegisterTargetManager(tmloader()); err != nil {
			log.Fatal(err)
		}
	}

	// Register TargetLocker plugins
	for _, tlloader := range targetLockers {
		if err := pluginRegistry.RegisterTargetLocker(tlloader()); err != nil {
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
	log.Infof("Using database URI (MySQL DSN) of the main storage: %s", *flagDBURI)
	s, err := rdbms.New(*flagDBURI)
	if err != nil {
		log.Fatalf("could not initialize database: %v", err)
	}
	storage.SetStorage(s)

	// set Locker engine
	targetLocker, targetLockerName, targetLockerStorageDSN := newTargetLockerFromFlag(*flagTargetLocker)
	log.Infof("Using target locker '%s' with DSN: '%s'", targetLockerName, targetLockerStorageDSN)
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
