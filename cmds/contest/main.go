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
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"

	"github.com/facebookincubator/contest/plugins/targetlocker/dblocker"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"

	"github.com/sirupsen/logrus"
)

var (
	flagDBURI          = flag.String("dbURI", config.DefaultDBURI, "Database URI")
	flagServerID       = flag.String("serverID", "", "Set a static server ID, e.g. the host name or another unique identifier. If unset, will use the listener's default")
	flagProcessTimeout = flag.Duration("processTimeout", api.DefaultEventTimeout, "API request processing timeout")
	flagTargetLocker   = flag.String("targetLocker", inmemory.Name, "Target locker implementation to use")
	flagInstanceTag    = flag.String("instanceTag", "", "A tag for this instance. Server will only operate on jobs with this tag and will add this tag to the jobs it creates.")
)

func main() {
	flag.Parse()
	log := logging.GetLogger("contest")
	log.Level = logrus.DebugLevel
	logging.Debug()

	pluginRegistry := pluginregistry.NewPluginRegistry()

	// primary storage initialization
	if *flagDBURI != "" {
		log.Infof("Using database URI for primary storage: %s", *flagDBURI)

		primaryDBURI := *flagDBURI
		s, err := rdbms.New(primaryDBURI)
		if err != nil {
			log.Fatalf("Could not initialize database: %v", err)
		}
		if err := storage.SetStorage(s); err != nil {
			log.Fatalf("Could not set storage: %v", err)
		}

		dbVerPrim, err := s.Version()
		if err != nil {
			log.Warningf("Could not determine storage version: %v", err)
		} else {
			log.Infof("Storage version: %d", dbVerPrim)
		}

		// replica storage initialization
		log.Infof("Using database URI for replica storage: %s", *flagDBURI)

		// pointing to main database for now but can be used to point to replica
		replicaDBURI := *flagDBURI
		r, err := rdbms.New(replicaDBURI)
		if err != nil {
			log.Fatalf("Could not initialize replica database: %v", err)
		}
		if err := storage.SetAsyncStorage(r); err != nil {
			log.Fatalf("Could not set replica storage: %v", err)
		}

		dbVerRepl, err := r.Version()
		if err != nil {
			log.Warningf("Could not determine storage version: %v", err)
		} else {
			log.Infof("Storage version: %d", dbVerRepl)
		}

		if dbVerPrim != dbVerRepl {
			log.Fatalf("Primary and Replica DB Versions are different: %v and %v", dbVerPrim, dbVerRepl)
		}
	} else {
		log.Warningf("Using in-memory storage")
		if ms, err := memory.New(); err == nil {
			if err := storage.SetStorage(ms); err != nil {
				log.Fatalf("Could not set storage: %v", err)
			}
			if err := storage.SetAsyncStorage(ms); err != nil {
				log.Fatalf("Could not set replica storage: %v", err)
			}
		} else {
			log.Fatalf("Could not create storage: %v", err)
		}
	}

	// set Locker engine
	switch *flagTargetLocker {
	case inmemory.Name:
		target.SetLocker(inmemory.New())
	case dblocker.Name:
		if l, err := dblocker.New(*flagDBURI); err == nil {
			target.SetLocker(l)
		} else {
			log.Fatalf("Failed to create locker %q: %v", *flagTargetLocker, err)
		}
	default:
		log.Fatalf("Invalid target locker name %q", *flagTargetLocker)
	}

	plugins.Init(pluginRegistry, log)

	// spawn JobManager
	listener := httplistener.HTTPListener{}

	opts := []jobmanager.Option{
		jobmanager.APIOption(api.OptionEventTimeout(*flagProcessTimeout)),
	}
	if *flagServerID != "" {
		opts = append(opts, jobmanager.APIOption(api.OptionServerID(*flagServerID)))
	}
	if *flagInstanceTag != "" {
		opts = append(opts, jobmanager.OptionInstanceTag(*flagInstanceTag))
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
