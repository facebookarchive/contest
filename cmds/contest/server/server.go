// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package server

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/userfunctions/donothing"
	"github.com/facebookincubator/contest/pkg/userfunctions/ocp"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
	"github.com/facebookincubator/contest/plugins/targetlocker/dblocker"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"

	// the listener plugin
	"github.com/facebookincubator/contest/plugins/listeners/httplistener"
)

var (
	flagSet                *flag.FlagSet
	flagDBURI              *string
	flagListenAddr         *string
	flagServerID           *string
	flagProcessTimeout     *time.Duration
	flagTargetLocker       *string
	flagInstanceTag        *string
	flagLogLevel           *string
	flagPauseTimeout       *time.Duration
	flagResumeJobs         *bool
	flagTargetLockDuration *time.Duration
)

func initFlags(cmd string) {
	flagSet = flag.NewFlagSet(cmd, flag.ContinueOnError)
	flagDBURI = flagSet.String("dbURI", config.DefaultDBURI, "Database URI")
	flagListenAddr = flagSet.String("listenAddr", ":8080", "Listen address and port")
	flagServerID = flagSet.String("serverID", "", "Set a static server ID, e.g. the host name or another unique identifier. If unset, will use the listener's default")
	flagProcessTimeout = flagSet.Duration("processTimeout", api.DefaultEventTimeout, "API request processing timeout")
	flagTargetLocker = flagSet.String("targetLocker", "auto", "Target locker implementation to use, \"auto\" follows DBURI setting")
	flagInstanceTag = flagSet.String("instanceTag", "", "A tag for this instance. Server will only operate on jobs with this tag and will add this tag to the jobs it creates.")
	flagLogLevel = flagSet.String("logLevel", "debug", "A log level, possible values: debug, info, warning, error, panic, fatal")
	flagPauseTimeout = flagSet.Duration("pauseTimeout", 0, "SIGINT/SIGTERM shutdown timeout (seconds), after which pause will be escalated to cancellaton; -1 - no escalation, 0 - do not pause, cancel immediately")
	flagResumeJobs = flagSet.Bool("resumeJobs", false, "Attempt to resume paused jobs")
	flagTargetLockDuration = flagSet.Duration("targetLockDuration", config.DefaultTargetLockDuration,
		"The amount of time target lock is extended by while the job is running. "+
			"This is the maximum amount of time a job can stay paused safely.")
}

var userFunctions = []map[string]interface{}{
	ocp.Load(),
	donothing.Load(),
}

var funcInitOnce sync.Once

// PluginConfig contains the configuration for all the plugins desired in a
// server instance.
type PluginConfig struct {
	TargetManagerLoaders []target.TargetManagerLoader
	TestFetcherLoaders   []test.TestFetcherLoader
	TestStepLoaders      []test.TestStepLoader
	ReporterLoaders      []job.ReporterLoader
}

func registerPlugins(pluginRegistry *pluginregistry.PluginRegistry, pluginConfig *PluginConfig) error {
	// register targetmanagers
	for _, loader := range pluginConfig.TargetManagerLoaders {
		name, factory := loader()
		if err := pluginRegistry.RegisterTargetManager(name, factory); err != nil {
			return err
		}
	}

	// register testfetchers
	for _, loader := range pluginConfig.TestFetcherLoaders {
		name, factory := loader()
		if err := pluginRegistry.RegisterTestFetcher(name, factory); err != nil {
			return err
		}
	}

	// register teststeps
	for _, loader := range pluginConfig.TestStepLoaders {
		name, factory, events := loader()
		if err := pluginRegistry.RegisterTestStep(name, factory, events); err != nil {
			return err
		}
	}

	// register reporters
	for _, loader := range pluginConfig.ReporterLoaders {
		name, factory := loader()
		if err := pluginRegistry.RegisterReporter(name, factory); err != nil {
			return err
		}
	}

	// TODO make listener also configurable from contest-generator.
	// also register user functions here. TODO: make them configurable from contest-generator.
	errCh := make(chan error, 1)
	funcInitOnce.Do(func() {
		for _, userFunction := range userFunctions {
			for name, fn := range userFunction {
				if err := test.RegisterFunction(name, fn); err != nil {
					errCh <- fmt.Errorf("failed to load user function '%s': %w", name, err)
					return
				}
			}
		}
	})

	return nil
}

// Main is the main function that executes the ConTest server.
func Main(pluginConfig *PluginConfig, cmd string, args []string, sigs <-chan os.Signal) error {
	initFlags(cmd)
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	logLevel, err := logger.ParseLogLevel(*flagLogLevel)
	if err != nil {
		return err
	}

	clk := clock.New()

	ctx, cancel := xcontext.WithCancel(logrusctx.NewContext(logLevel, logging.DefaultOptions()...))
	ctx, pause := xcontext.WithNotify(ctx, xcontext.ErrPaused)
	log := ctx.Logger()
	defer cancel()

	pluginRegistry := pluginregistry.NewPluginRegistry(ctx)
	if err := registerPlugins(pluginRegistry, pluginConfig); err != nil {
		return fmt.Errorf("failed to register plugins: %w", err)
	}

	var storageInstances []storage.Storage
	defer func() {
		for i, s := range storageInstances {
			if err := s.Close(); err != nil {
				log.Errorf("Failed to close storage %d: %v", i, err)
			}
		}
	}()

	// primary storage initialization
	if *flagDBURI != "" {
		primaryDBURI := *flagDBURI
		log.Infof("Using database URI for primary storage: %s", primaryDBURI)
		s, err := rdbms.New(primaryDBURI)
		if err != nil {
			log.Fatalf("Could not initialize database: %v", err)
		}
		storageInstances = append(storageInstances, s)
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
		// pointing to main database for now but can be used to point to replica
		replicaDBURI := *flagDBURI
		log.Infof("Using database URI for replica storage: %s", replicaDBURI)
		r, err := rdbms.New(replicaDBURI)
		if err != nil {
			log.Fatalf("Could not initialize replica database: %v", err)
		}
		storageInstances = append(storageInstances, r)
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
			storageInstances = append(storageInstances, ms)
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
	if *flagTargetLocker == "auto" {
		if *flagDBURI != "" {
			*flagTargetLocker = dblocker.Name
		} else {
			*flagTargetLocker = inmemory.Name
		}
		log.Infof("Locker engine set to auto, using %s", *flagTargetLocker)
	}
	switch *flagTargetLocker {
	case inmemory.Name:
		target.SetLocker(inmemory.New(clk))
	case dblocker.Name:
		if l, err := dblocker.New(*flagDBURI, dblocker.WithClock(clk)); err == nil {
			target.SetLocker(l)
		} else {
			log.Fatalf("Failed to create locker %q: %v", *flagTargetLocker, err)
		}
	default:
		log.Fatalf("Invalid target locker name %q", *flagTargetLocker)
	}

	// spawn JobManager
	listener := httplistener.New(*flagListenAddr)

	opts := []jobmanager.Option{
		jobmanager.APIOption(api.OptionEventTimeout(*flagProcessTimeout)),
	}
	if *flagServerID != "" {
		opts = append(opts, jobmanager.APIOption(api.OptionServerID(*flagServerID)))
	}
	if *flagInstanceTag != "" {
		opts = append(opts, jobmanager.OptionInstanceTag(*flagInstanceTag))
	}
	if *flagTargetLockDuration != 0 {
		opts = append(opts, jobmanager.OptionTargetLockDuration(*flagTargetLockDuration))
	}

	jm, err := jobmanager.New(listener, pluginRegistry, opts...)
	if err != nil {
		log.Fatalf("%v", err)
	}

	pauseTimeout := *flagPauseTimeout

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
							case <-time.After(pauseTimeout):
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

	err = jm.Run(ctx, *flagResumeJobs)

	target.SetLocker(nil)
	_ = storage.SetStorage(nil)
	_ = storage.SetAsyncStorage(nil)

	log.Infof("Exiting, %v", err)

	return err
}
