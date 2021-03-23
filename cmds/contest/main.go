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
	"time"

	"github.com/facebookincubator/contest/cmds/plugins"
	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/plugins/listeners/httplistener"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
	"github.com/facebookincubator/contest/plugins/targetlocker/dblocker"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"
)

var (
	flagDBURI          = flag.String("dbURI", config.DefaultDBURI, "Database URI")
	flagServerID       = flag.String("serverID", "", "Set a static server ID, e.g. the host name or another unique identifier. If unset, will use the listener's default")
	flagProcessTimeout = flag.Duration("processTimeout", api.DefaultEventTimeout, "API request processing timeout")
	flagTargetLocker   = flag.String("targetLocker", inmemory.Name, "Target locker implementation to use")
	flagInstanceTag    = flag.String("instanceTag", "", "A tag for this instance. Server will only operate on jobs with this tag and will add this tag to the jobs it creates.")
	flagLogLevel       = flag.String("logLevel", "debug", "A log level, possible values: debug, info, warning, error, panic, fatal")
	flagPauseTimeout   = flag.Int("pauseTimeout", 0, "SIGINT/SIGTERM shutdown timeout (seconds), after which pause will be escalated to cancellaton; -1 - no escalation, 0 - do not pause, cancel immediately")
	flagResumeJobs     = flag.Bool("resumeJobs", false, "Attempt to resume paused jobs")
)

func main() {
	flag.Parse()
	logLevel, err := logger.ParseLogLevel(*flagLogLevel)
	if err != nil {
		panic(err)
	}

	ctx, cancel := xcontext.WithCancel(logrusctx.NewContext(logLevel, logging.DefaultOptions()...))
	ctx2, pause := xcontext.WithNotify(ctx, xcontext.ErrPaused)
	ctx = ctx2
	log := ctx.Logger()

	pluginRegistry := pluginregistry.NewPluginRegistry(ctx)

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
			log.Warnf("Could not determine storage version: %v", err)
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
			log.Warnf("Could not determine storage version: %v", err)
		} else {
			log.Infof("Storage version: %d", dbVerRepl)
		}

		if dbVerPrim != dbVerRepl {
			log.Fatalf("Primary and Replica DB Versions are different: %v and %v", dbVerPrim, dbVerRepl)
		}
	} else {
		log.Warnf("Using in-memory storage")
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

	plugins.Init(pluginRegistry, ctx.Logger())

	// spawn JobManager
	listener := httplistener.NewHTTPListener()

	opts := []jobmanager.Option{
		jobmanager.APIOption(api.OptionEventTimeout(*flagProcessTimeout)),
	}
	if *flagServerID != "" {
		opts = append(opts, jobmanager.APIOption(api.OptionServerID(*flagServerID)))
	}
	if *flagInstanceTag != "" {
		opts = append(opts, jobmanager.OptionInstanceTag(*flagInstanceTag))
	}

	jm, err := jobmanager.New(listener, pluginRegistry, opts...)
	if err != nil {
		log.Fatalf("%v", err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	go func() {
		intLevel := 0
		// cancel immediately if pauseTimeout is zero
		if *flagPauseTimeout == 0 {
			intLevel = 1
		}
		for {
			sig, ok := <-sigs
			if !ok {
				return
			}
			switch sig {
			case syscall.SIGUSR1:
				// Gentle shutdown: stop accepting requests, drain without asserting pause signal.
				jm.StopAPI()
			case syscall.SIGINT:
				fallthrough
			case syscall.SIGTERM:
				// First signal - pause and drain, second - cancel.
				jm.StopAPI()
				if intLevel == 0 {
					log.Infof("Signal %q, pausing jobs", sig)
					pause()
					if *flagPauseTimeout > 0 {
						go func() {
							select {
							case <-ctx.Done():
							case <-time.After(time.Duration(*flagPauseTimeout) * time.Second):
								log.Errorf("Timed out waiting for jobs to pause, canceling")
								cancel()
							}
						}()
					}
					intLevel++
				} else {
					log.Infof("Signal %q, canceling", sig)
					cancel()
				}
			}
		}
	}()

	if err := jm.Run(ctx, *flagResumeJobs); err != nil {
		log.Fatalf("%v", err)
	}
	close(sigs)
	log.Infof("Exiting")
}
