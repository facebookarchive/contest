// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/insomniacslk/xjson"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/runner"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

var cancellationTimeout = 60 * time.Second

// ErrorEventPayload represents the payload carried by a failure event (e.g. JobStateFailed, JobStateCancelled, etc.)
type ErrorEventPayload struct {
	Err xjson.Error
}

// JobManager is the core component for the long-running job management service.
// It handles API requests, test fetching, target fetching, and jobs lifecycle.
//
// In more detail, it is responsible for:
// * spawning the API listener, and handling the incoming requests
// * fetching targets, via target managers
// * fetching test definitions, via test fetchers
// * enqueuing new job requests, and handling their status
// * starting, stopping, and retrying jobs
type JobManager struct {
	config

	jobs      map[types.JobID]*job.Job
	jobRunner *runner.JobRunner

	jobsMu sync.Mutex
	jobsWg sync.WaitGroup

	jobStorageManager storage.JobStorageManager

	frameworkEvManager frameworkevent.EmitterFetcher
	testEvManager      testevent.Fetcher

	apiListener    api.Listener
	pluginRegistry *pluginregistry.PluginRegistry
}

// New initializes and returns a new JobManager with the given API listener.
func New(l api.Listener, pr *pluginregistry.PluginRegistry, opts ...Option) (*JobManager, error) {
	if pr == nil {
		return nil, errors.New("plugin registry cannot be nil")
	}
	jobStorageManager := storage.NewJobStorageManager()

	frameworkEvManager := storage.NewFrameworkEventEmitterFetcher()
	testEvManager := storage.NewTestEventFetcher()

	cfg := getConfig(opts...)
	if cfg.instanceTag != "" {
		if err := job.IsValidTag(cfg.instanceTag, true /* allowInternal */); err != nil {
			return nil, fmt.Errorf("invalid instaceTag: %w", err)
		}
		if !job.IsInternalTag(cfg.instanceTag) {
			return nil, fmt.Errorf("instaceTag must be an internal tag (start with %q)", job.InternalTagPrefix)
		}
	}

	jm := JobManager{
		config:             cfg,
		apiListener:        l,
		pluginRegistry:     pr,
		jobs:               make(map[types.JobID]*job.Job),
		jobStorageManager:  jobStorageManager,
		frameworkEvManager: frameworkEvManager,
		testEvManager:      testEvManager,
	}
	jm.jobRunner = runner.NewJobRunner()
	return &jm, nil
}

func (jm *JobManager) handleEvent(ev *api.Event) {
	var resp *api.EventResponse

	switch ev.Type {
	case api.EventTypeStart:
		resp = jm.start(ev)
	case api.EventTypeStatus:
		resp = jm.status(ev)
	case api.EventTypeStop:
		resp = jm.stop(ev)
	case api.EventTypeRetry:
		resp = jm.retry(ev)
	case api.EventTypeList:
		resp = jm.list(ev)
	default:
		resp = &api.EventResponse{
			Requestor: ev.Msg.Requestor(),
			Err:       fmt.Errorf("invalid event type: %v", ev.Type),
		}
	}

	ev.Context.Logger().Debugf("Sending response %+v", resp)
	// time to wait before printing an error if the response is not received.
	sendEventTimeout := 3 * time.Second

	select {
	case ev.RespCh <- resp:
	case <-time.After(sendEventTimeout):
		// TODO send failure event once we have the event infra
		// TODO determine whether the server should shut down if there
		//      are too many errors
		ev.Context.Logger().Panicf("timed out after %v trying to send a response event", sendEventTimeout)
	}
}

// Start is responsible for starting the API listener and responding to incoming
// events. It also responds to cancellation requests coming from SIGINT/SIGTERM
// signals, propagating the signals downwards to all jobs.
func (jm *JobManager) Start(ctx xcontext.Context, sigs chan os.Signal) error {
	log := ctx.Logger()

	a, err := api.New(ctx, jm.config.apiOptions...)
	if err != nil {
		return fmt.Errorf("Cannot start JobManager: %w", err)
	}

	apiCtx, apiCancel := xcontext.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		lErr := jm.apiListener.Serve(apiCtx, a)
		ctx.Logger().Debugf("Server shut down successfully.")
		errCh <- lErr
	}()

loop:
	for {
		select {
		// handle events from the API
		case ev := <-a.Events:
			ev.Context.Logger().Debugf("Handling event %+v", ev)
			// send the response, and wait for the given timeout
			jm.handleEvent(ev)
		// check for errors or premature termination from the listener.
		case err := <-errCh:
			log.Infof("JobManager: API listener failed, triggering a cancellation of all jobs")
			jm.CancelAll(ctx)
			if err != nil {
				return fmt.Errorf("error reported by API listener: %v", err)
			}
			return errors.New("API listener terminated prematurely without errors")
		// handle signals to shut down gracefully. If the cancellation takes too
		// long, it will be terminated.
		case sig := <-sigs:
			// TODO: stop processing signals inside jobmanager, this is
			//       as responsibility of "main".
			// We were interrupted by a signal, time to leave!
			if sig == syscall.SIGUSR1 {
				log.Debugf("Interrupted by signal '%s': wait for jobs and exit", sig)
				apiCancel()
			} else {
				log.Debugf("Interrupted by signal '%s': pause jobs and exit", sig)
				apiCancel()
				jm.PauseJobs(ctx)
			}
			select {
			case err := <-errCh:
				if err != nil {
					return fmt.Errorf("API listener terminated with error: %v", err)
				}
				// break the outer loop
				break loop
			case <-time.After(cancellationTimeout):
				return fmt.Errorf("API listener didn't shut down within %v, exiting", cancellationTimeout)
			}
		}
	}
	// Downstream runner are guaranteed to have shutdown control path protected
	// by timeouts, therefore here we can wait for all jobs registered in the
	// WaitGroup to terminate correctly or timeout during termination
	jm.jobsWg.Wait()
	return nil
}

// CancelJob sends a cancellation request to a specific job.
func (jm *JobManager) CancelJob(jobID types.JobID) error {
	jm.jobsMu.Lock()
	// get the job from the local cache rather than the storage layer. We can
	// only cancel jobs that we are actively handling.
	j, ok := jm.jobs[jobID]
	if !ok {
		jm.jobsMu.Unlock()
		return fmt.Errorf("unknown job ID: %d", jobID)
	}
	j.Cancel()
	delete(jm.jobs, jobID)
	jm.jobsMu.Unlock()
	return nil
}

// CancelAll sends a cancellation request to the API listener and to every running
// job.
func (jm *JobManager) CancelAll(ctx xcontext.Context) {
	log := ctx.Logger()
	// TODO This doesn't seem the right thing to do, if the listener fails we should
	// pause, not cancel.

	// Get the job from the local cache rather than the storage layer. We can
	// only cancel jobs that we are actively handling.
	log.Infof("JobManager: cancelling all jobs")
	for jobID, job := range jm.jobs {
		log.Debugf("JobManager: cancelling job with ID %v", jobID)
		job.Cancel()
	}
}

// PauseJobs sends a pause request to every running job.
func (jm *JobManager) PauseJobs(ctx xcontext.Context) {
	log := ctx.Logger()

	log.Infof("JobManager: requested pausing")
	for jobID, job := range jm.jobs {
		log.Debugf("JobManager: pausing job with ID %v", jobID)
		job.Pause()
	}
}

func (jm *JobManager) emitErrEvent(ctx xcontext.Context, jobID types.JobID, eventName event.Name, err error) error {
	var (
		rawPayload json.RawMessage
		payloadPtr *json.RawMessage
	)
	if err != nil {
		ctx.Logger().Errorf(err.Error())
		payload := ErrorEventPayload{Err: *xjson.NewError(err)}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			ctx.Logger().Warnf("Could not serialize payload for event %s: %v", eventName, err)
		} else {
			rawPayload = json.RawMessage(payloadJSON)
			payloadPtr = &rawPayload
		}
	}

	ev := frameworkevent.Event{
		JobID:     jobID,
		EventName: eventName,
		Payload:   payloadPtr,
		EmitTime:  time.Now(),
	}
	if err := jm.frameworkEvManager.Emit(ctx, ev); err != nil {
		ctx.Logger().Warnf("Could not emit event %s for job %d: %v", eventName, jobID, err)
		return err
	}
	return nil
}

func (jm *JobManager) emitEvent(ctx xcontext.Context, jobID types.JobID, eventName event.Name) error {
	return jm.emitErrEvent(ctx, jobID, eventName, nil)
}
