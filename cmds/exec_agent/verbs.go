// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/facebookincubator/contest/pkg/xcontext"
)

const (
	ProcessFinishedExitCode = 13
	DeadAgentExitCode       = 14
)

func run() error {
	verb := strings.ToLower(flagSet.Arg(0))
	if verb == "" {
		return fmt.Errorf("missing verb, see --help")
	}

	switch verb {
	case "start":
		bin := flagSet.Arg(1)
		if bin == "" {
			return fmt.Errorf("missing binary argument, see --help")
		}

		var args []string
		if flagSet.NArg() > 2 {
			args = flagSet.Args()[2:]
		}
		return start(bin, args)

	case "poll":
		pid, err := strconv.Atoi(flagSet.Arg(1))
		if err != nil {
			return fmt.Errorf("failed to parse exec id: %w", err)
		}

		return poll(pid)

	case "wait":
		pid, err := strconv.Atoi(flagSet.Arg(1))
		if err != nil {
			return fmt.Errorf("failed to parse exec id: %w", err)
		}

		return wait(pid)

	default:
		return fmt.Errorf("invalid verb: %s", verb)
	}
}

func start(bin string, args []string) error {
	ctx := xcontext.Background()

	// signal channel for process exit. Similar to Wait
	reaper := newSafeSignal()
	defer reaper.Signal()

	if flagTimeQuota != nil && *flagTimeQuota != 0 {
		var cancel xcontext.CancelFunc
		ctx, cancel = xcontext.WithTimeout(ctx, *flagTimeQuota)
		defer cancel()

		// skip waiting for remote to poll/wait
		go func() {
			<-ctx.Done()
			reaper.Signal()
		}()
	}

	cmd := exec.CommandContext(ctx, bin, args...)

	// TODO: race condition on these buffers; fix
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("starting command: %s", cmd.String())
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}
	// NOTE: write this only pid on stdout
	fmt.Printf("%d\n", cmd.Process.Pid)

	cmdErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("process exited with err: %v", err)
		} else {
			log.Print("process finished")
		}

		reaper.Wait()
		cmdErr <- err
	}()

	// start unix socket server
	mon := NewMonitorServer(cmd.Process, &stdout, &stderr, reaper)
	monErr := make(chan error, 1)
	go func() {
		monErr <- mon.Serve()
	}()

	// catch termination signals
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case err := <-cmdErr:
			if err := mon.Shutdown(); err != nil {
				log.Printf("failed to shutdown monitor: %v", err)
			}
			return err

		case sig := <-sigs:
			if err := mon.Shutdown(); err != nil {
				log.Printf("failed to shutdown monitor: %v", err)
			}
			if cmd.ProcessState == nil {
				if err := cmd.Process.Kill(); err != nil {
					log.Printf("failed to kill process on signal: %v", err)
				}
			}
			return fmt.Errorf("signal caught: %v", sig)

		case err := <-monErr:
			if cmd.ProcessState == nil {
				if err := cmd.Process.Kill(); err != nil {
					log.Printf("failed to kill process on monitor error: %v", err)
				}
			}
			return err
		}
	}
}

func wait(pid int) error {
	mon := NewMonitorClient(pid)
	return mon.Wait()
}

func poll(pid int) error {
	mon := NewMonitorClient(pid)

	data, err := mon.Poll()
	if err != nil {
		// connection errors also means that the process or agent might have died
		var e *ErrCantConnect
		if errors.As(err, &e) {
			os.Exit(DeadAgentExitCode)
		}

		return fmt.Errorf("failed to call monitor: %w", err)
	}

	fmt.Printf("%s", string(data.Stdout))
	fmt.Fprintf(os.Stderr, "%s", string(data.Stderr))
	if !data.Alive {
		os.Exit(ProcessFinishedExitCode)
	}

	return nil
}
