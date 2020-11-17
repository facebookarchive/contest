// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/facebookincubator/contest/cmds/plugins"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"

	"github.com/facebookincubator/contest/plugins/listeners/httplistener"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"

	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"

	"github.com/sirupsen/logrus"
)

var (
	flagDBURI          = flag.String("dbURI", config.DefaultDBURI, "Database URI")
	flagServerID       = flag.String("serverID", "", "Set a static server ID, e.g. the host name or another unique identifier. If unset, will use the listener's default")
	flagProcessTimeout = flag.Duration("processTimeout", api.DefaultEventTimeout, "API request processing timeout")
)

func main() {
	flag.Parse()
	log := logging.GetLogger("contest")
	log.Level = logrus.DebugLevel

	pluginRegistry := pluginregistry.NewPluginRegistry()

	// storage initialization
	log.Infof("Using database URI: %s", *flagDBURI)
	s, err := rdbms.New(*flagDBURI)
	if err != nil {
		log.Fatalf("could not initialize database: %v", err)
	}
	storage.SetStorage(s)

	dbVer, err := s.Version()
	if err != nil {
		log.Warningf("could not determine storage version: %v", err)
	} else {
		log.Infof("storage version: %d", dbVer)
	}

	// set Locker engine
	target.SetLocker(inmemory.New(config.LockInitialTimeout, config.LockRefreshTimeout))

	plugins.Init(pluginRegistry, log)

	// spawn JobManager
	listener := httplistener.HTTPListener{}

	opts := []jobmanager.Option{
		jobmanager.APIOption(api.OptionEventTimeout(*flagProcessTimeout)),
	}
	if *flagServerID != "" {
		opts = append(opts, jobmanager.APIOption(api.OptionServerID(*flagServerID)))
	}

	jm, err := jobmanager.New(&listener, pluginRegistry, opts...)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("JobManager %+v", jm)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	if err := jm.Start(sigs); err != nil {
		log.Fatal(err)
	}
}
