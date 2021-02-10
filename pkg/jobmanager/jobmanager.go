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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/insomniacslk/xjson"

	"github.com/facebookincubator/contest/pkg/api"
	pkg_config "github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/runner"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/storage/limits"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

var log = logging.GetLogger("pkg/jobmanager")

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
	apiCancel      chan struct{}
	pluginRegistry *pluginregistry.PluginRegistry
}

// NewJobFromRequest returns a new Job object from a job.Request .
func NewJobFromRequest(pr *pluginregistry.PluginRegistry, req *job.Request) (*job.Job, error) {
	var jd *job.JobDescriptor
	if err := json.Unmarshal([]byte(req.JobDescriptor), &jd); err != nil {
		return nil, err
	}
	j, err := newPartialJobFromDescriptor(pr, jd)
	if err != nil {
		return nil, err
	}
	// TODO set j.RunReporterBundles and j.FinalReporterBundles
	return j, nil
}

func newPartialJobFromDescriptor(pr *pluginregistry.PluginRegistry, jd *job.JobDescriptor) (*job.Job, error) {

	if jd == nil {
		return nil, errors.New("JobDescriptor cannot be nil")
	}
	if len(jd.TestDescriptors) == 0 {
		return nil, errors.New("need at least one TestDescriptor in the JobDescriptor")
	}
	if jd.JobName == "" {
		return nil, errors.New("job name cannot be empty")
	}
	if err := limits.NewValidator().ValidateJobName(jd.JobName); err != nil {
		return nil, err
	}
	if jd.RunInterval < 0 {
		return nil, errors.New("run interval must be non-negative")
	}

	if len(jd.Reporting.RunReporters) == 0 && len(jd.Reporting.FinalReporters) == 0 {
		return nil, errors.New("at least one run reporter or one final reporter must be specified in a job")
	}
	for _, reporter := range jd.Reporting.RunReporters {
		if strings.TrimSpace(reporter.Name) == "" {
			return nil, errors.New("run reporters cannot have empty or all-whitespace names")
		}
		if err := limits.NewValidator().ValidateReporterName(reporter.Name); err != nil {
			return nil, err
		}
	}

	tests := make([]*test.Test, 0, len(jd.TestDescriptors))
	testDescriptors := make([][]*test.TestStepDescriptor, 0, len(jd.TestDescriptors))
	for _, td := range jd.TestDescriptors {
		if td == nil {
			return nil, errors.New("test description is null")
		}
		if td.TargetManagerName == "" {
			return nil, errors.New("target manager name cannot be empty")
		}
		if td.TestFetcherName == "" {
			return nil, errors.New("test fetcher name cannot be empty")
		}
		// get an instance of the TargetManager and validate its parameters.
		tmb, err := pr.NewTargetManagerBundle(td)
		if err != nil {
			return nil, err
		}
		// get an instance of the TestFetcher and validate its parameters
		tfb, err := pr.NewTestFetcherBundle(td)
		if err != nil {
			return nil, err
		}
		name, testStepDescs, err := tfb.TestFetcher.Fetch(tfb.FetchParameters)
		if err != nil {
			return nil, err
		}
		if err := limits.NewValidator().ValidateTestName(name); err != nil {
			return nil, err
		}
		testDescriptors = append(testDescriptors, testStepDescs)

		// look up test step plugins in the plugin registry
		var stepBundles []test.TestStepBundle
		labels := make(map[string]bool)
		for idx, testStepDesc := range testStepDescs {
			if testStepDesc == nil {
				return nil, errors.New("test step description is null")
			}
			if err := limits.NewValidator().ValidateTestStepLabel(testStepDesc.Label); err != nil {
				return nil, err
			}
			tse, err := pr.NewTestStepEvents(testStepDesc.Name)
			if err != nil {
				return nil, err
			}
			tsb, err := pr.NewTestStepBundle(*testStepDesc, tse)
			if err != nil {
				return nil, fmt.Errorf("NewTestStepBundle for test step '%s' with index %d failed: %w", testStepDesc.Name, idx, err)
			}
			if _, ok := labels[tsb.TestStepLabel]; ok {
				// validate that the label associated to the test step does not clash
				// with any other label within the test
				return nil, fmt.Errorf("found duplicated labels in test %s: %s ", name, tsb.TestStepLabel)
			}
			labels[tsb.TestStepLabel] = true

			if err != nil {
				return nil, err
			}
			stepBundles = append(stepBundles, *tsb)
		}
		test := test.Test{
			Name:                name,
			TargetManagerBundle: tmb,
			TestFetcherBundle:   tfb,
			TestStepsBundles:    stepBundles,
		}
		tests = append(tests, &test)
	}

	testDescriptorsJSON, err := json.Marshal(testDescriptors)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal test descriptors: %w", err)
	}

	var targetManagerAcquireTimeout = pkg_config.TargetManagerAcquireTimeout
	if jd.TargetManagerAcquireTimeout != nil {
		targetManagerAcquireTimeout = time.Duration(*jd.TargetManagerAcquireTimeout)
	}

	var targetManagerReleaseTimeout = pkg_config.TargetManagerReleaseTimeout
	if jd.TargetManagerReleaseTimeout != nil {
		targetManagerReleaseTimeout = time.Duration(*jd.TargetManagerReleaseTimeout)
	}

	j := job.Job{
		ID:          types.JobID(0),
		Name:        jd.JobName,
		Tags:        jd.Tags,
		Runs:        jd.Runs,
		RunInterval: time.Duration(jd.RunInterval),
		TargetManagerAcquireTimeout: targetManagerAcquireTimeout,
		TargetManagerReleaseTimeout: targetManagerReleaseTimeout,
		// reporter bundles must be set externally
		TestDescriptors:      string(testDescriptorsJSON),
		Tests:                tests,
		RunReporterBundles:   nil,
		FinalReporterBundles: nil,
	}

	j.StateCtx, j.StateCtxPause, j.StateCtxCancel = statectx.New()
	return &j, nil
}

// NewJob returns a new Job object and the fetched test descriptors
func NewJob(pr *pluginregistry.PluginRegistry, jobDescriptor string) (*job.Job, error) {
	var jd *job.JobDescriptor
	if err := json.Unmarshal([]byte(jobDescriptor), &jd); err != nil {
		return nil, err
	}
	j, err := newPartialJobFromDescriptor(pr, jd)
	if err != nil {
		return nil, err
	}

	var runReporterBundles []*job.ReporterBundle
	for _, reporter := range jd.Reporting.RunReporters {
		if strings.TrimSpace(reporter.Name) == "" {
			return nil, errors.New("invalid empty or all-whitespace run reporter name")
		}
		bundle, err := pr.NewRunReporterBundle(reporter.Name, reporter.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to create bundle for run reporter '%s': %v", reporter.Name, err)
		}
		runReporterBundles = append(runReporterBundles, bundle)
	}
	var finalReporterBundles []*job.ReporterBundle
	for _, reporter := range jd.Reporting.FinalReporters {
		if strings.TrimSpace(reporter.Name) == "" {
			return nil, errors.New("invalid empty or all-whitespace final reporter name")
		}
		bundle, err := pr.NewFinalReporterBundle(reporter.Name, reporter.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to create bundle for final reporter '%s': %v", reporter.Name, err)
		}
		finalReporterBundles = append(finalReporterBundles, bundle)
	}

	j.RunReporterBundles = runReporterBundles
	j.FinalReporterBundles = finalReporterBundles

	return j, nil
}

// New initializes and returns a new JobManager with the given API listener.
func New(l api.Listener, pr *pluginregistry.PluginRegistry, opts ...Option) (*JobManager, error) {
	if pr == nil {
		return nil, errors.New("plugin registry cannot be nil")
	}
	jobStorageManager := storage.NewJobStorageManager()

	frameworkEvManager := storage.NewFrameworkEventEmitterFetcher()
	testEvManager := storage.NewTestEventFetcher()

	jm := JobManager{
		config:             getConfig(opts...),
		apiListener:        l,
		pluginRegistry:     pr,
		jobs:               make(map[types.JobID]*job.Job),
		jobStorageManager:  jobStorageManager,
		frameworkEvManager: frameworkEvManager,
		testEvManager:      testEvManager,
		apiCancel:          make(chan struct{}),
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

	log.Printf("Sending response %+v", resp)
	// time to wait before printing an error if the response is not received.
	sendEventTimeout := 3 * time.Second

	select {
	case ev.RespCh <- resp:
	case <-time.After(sendEventTimeout):
		// TODO send failure event once we have the event infra
		// TODO determine whether the server should shut down if there
		//      are too many errors
		log.Panicf("timed out after %v trying to send a response event", sendEventTimeout)
	}
}

// Start is responsible for starting the API listener and responding to incoming
// events. It also responds to cancellation requests coming from SIGINT/SIGTERM
// signals, propagating the signals downwards to all jobs.
func (jm *JobManager) Start(sigs chan os.Signal) error {
	a, err := api.New(jm.config.apiOptions...)
	if err != nil {
		return fmt.Errorf("Cannot start JobManager: %w", err)
	}
	errCh := make(chan error, 1)
	go func() {
		if lErr := jm.apiListener.Serve(jm.apiCancel, a); lErr != nil {
			errCh <- lErr
		}
		errCh <- nil
	}()
loop:
	for {
		select {
		// handle events from the API
		case ev := <-a.Events:
			log.Printf("Handling event %+v", ev)
			// send the response, and wait for the given timeout
			jm.handleEvent(ev)
		// check for errors or premature termination from the listener.
		case err := <-errCh:
			log.Info("JobManager: API listener failed, triggering a cancellation of all jobs")
			jm.CancelAll()
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
				log.Printf("Interrupted by signal '%s': wait for jobs and exit", sig)
				jm.stopAPI()
			} else {
				log.Printf("Interrupted by signal '%s': pause jobs and exit", sig)
				jm.stopAPI()
				jm.PauseJobs()
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
func (jm *JobManager) CancelAll() {
	// TODO This doesn't seem the right thing to do, if the listener fails we should
	// pause, not cancel.

	// Get the job from the local cache rather than the storage layer. We can
	// only cancel jobs that we are actively handling.
	log.Info("JobManager: cancelling all jobs")
	for jobID, job := range jm.jobs {
		log.Debugf("JobManager: cancelling job with ID %v", jobID)
		job.Cancel()
	}
}

func (jm *JobManager) stopAPI() {
	close(jm.apiCancel)
}

// PauseJobs sends a pause request to every running job.
func (jm *JobManager) PauseJobs() {
	log.Info("JobManager: requested pausing")
	for jobID, job := range jm.jobs {
		log.Debugf("JobManager: pausing job with ID %v", jobID)
		job.Pause()
	}
}

func (jm *JobManager) emitErrEvent(jobID types.JobID, eventName event.Name, err error) error {
	var (
		rawPayload json.RawMessage
		payloadPtr *json.RawMessage
	)
	if err != nil {
		log.Errorf(err.Error())
		payload := ErrorEventPayload{Err: *xjson.NewError(err)}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			log.Warningf("Could not serialize payload for event %s: %v", eventName, err)
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
	if err := jm.frameworkEvManager.Emit(ev); err != nil {
		log.Warningf("Could not emit event %s for job %d: %v", eventName, jobID, err)
		return err
	}
	return nil
}

func (jm *JobManager) emitEvent(jobID types.JobID, eventName event.Name) error {
	return jm.emitErrEvent(jobID, eventName, nil)
}
