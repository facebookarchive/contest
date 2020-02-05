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
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/runner"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
)

var log = logging.GetLogger("pkg/jobmanager")

var cancellationTimeout = 60 * time.Second

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
	jobs      map[types.JobID]*job.Job
	jobRunner *runner.JobRunner

	jobsMu sync.Mutex
	jobsWg sync.WaitGroup

	jobRequestManager  job.RequestEmitterFetcher
	jobReportManager   job.ReportEmitterFetcher
	frameworkEvManager frameworkevent.EmitterFetcher
	testEvManager      testevent.Fetcher

	apiListener    api.Listener
	apiCancel      chan struct{}
	pluginRegistry *pluginregistry.PluginRegistry
}

// NewJob creates a new Job object
func NewJob(pr *pluginregistry.PluginRegistry, jobDescriptor string) (*job.Job, error) {

	var jd *job.JobDescriptor
	if err := json.Unmarshal([]byte(jobDescriptor), &jd); err != nil {
		return nil, err
	}

	if jd == nil {
		return nil, errors.New("JobDescriptor cannot be nil")
	}
	if len(jd.TestDescriptors) == 0 {
		return nil, errors.New("need at least one TestDescriptor in the JobDescriptor")
	}
	if jd.JobName == "" {
		return nil, errors.New("job name cannot be empty")
	}
	if jd.RunInterval < 0 {
		return nil, errors.New("run interval must be non-negative")
	}
	// TODO(insomniacslk) enable multiple reporters after the reporter
	// refactoring
	if len(jd.Reporting.RunReporters) != 1 {
		return nil, errors.New("exactly one reporter must be specified in a job")
	}
	for _, reporter := range jd.Reporting.RunReporters {
		if strings.TrimSpace(reporter.Name) == "" {
			return nil, errors.New("reporters cannot have empty or all-whitespace names")
		}
	}

	reporterBundle, err := pr.NewReporterBundle(jd.Reporting.RunReporters[0].Name, jd.Reporting.RunReporters[0].Parameters)
	if err != nil {
		return nil, err
	}
	// TODO(insomniacslk) parse jd.Reporting.FinalReporters and add to the
	// bundle
	if len(jd.Reporting.FinalReporters) > 0 {
		return nil, errors.New("final reporters not supported yet")
	}

	tests := make([]*test.Test, 0, len(jd.TestDescriptors))
	for _, td := range jd.TestDescriptors {
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
		// look up test step plugins in the plugin registry
		var stepBundles []test.TestStepBundle
		labels := make(map[string]bool)
		for _, testStepDesc := range testStepDescs {
			tse, err := pr.NewTestStepEvents(testStepDesc.Name)
			if err != nil {
				return nil, err
			}
			tsb, err := pr.NewTestStepBundle(*testStepDesc, tse)
			if err != nil {
				return nil, fmt.Errorf("NewTestStepBundle for test step '%s' failed: %v", testStepDesc.Name, err)
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

	// Create a Job object from the above managers and parameters. The Job ID assigned
	// is 0, and gets actually set by the JobManager after calling the persistence layer
	job := job.Job{
		ID:             types.JobID(0),
		Name:           jd.JobName,
		Tags:           jd.Tags,
		Runs:           jd.Runs,
		RunInterval:    time.Duration(jd.RunInterval),
		Tests:          tests,
		ReporterBundle: reporterBundle,
	}

	job.Done = make(chan struct{})

	job.CancelCh = make(chan struct{})
	job.PauseCh = make(chan struct{})

	return &job, nil
}

// New initializes and returns a new JobManager with the given API listener.
func New(l api.Listener, pr *pluginregistry.PluginRegistry) (*JobManager, error) {
	if pr == nil {
		return nil, errors.New("plugin registry cannot be nil")
	}
	jobRequestManager := storage.NewJobRequestEmitterFetcher()
	jobReportManager := storage.NewJobReportEmitterFetcher()

	frameworkEvManager := storage.NewFrameworkEventEmitterFetcher()
	testEvManager := storage.NewTestEventFetcher()

	jm := JobManager{
		apiListener:        l,
		pluginRegistry:     pr,
		jobs:               make(map[types.JobID]*job.Job),
		jobRequestManager:  jobRequestManager,
		jobReportManager:   jobReportManager,
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
	a := api.New()
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
			// We were interrupted by a signal, time to leave!
			log.Printf("Interrupted by signal '%s', trying to exit gracefully", sig)
			jm.Pause()
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
	job, ok := jm.jobs[jobID]
	if !ok {
		jm.jobsMu.Unlock()
		return fmt.Errorf("unknown job ID: %d", jobID)
	}
	delete(jm.jobs, jobID)
	jm.jobsMu.Unlock()
	job.Cancel()
	return nil
}

// Cancel sends a cancellation request to the API listener and to every running
// job.
func (jm *JobManager) CancelAll() {
	// TODO This doesn't see the right thing to do, if the listener fails we should
	// pause, not cancel.
	log.Info("JobManager: cancelling all jobs")
	close(jm.apiCancel)
	for jobID, job := range jm.jobs {
		log.Debugf("JobManager: cancelling job with ID %v", jobID)
		job.Cancel()
	}
}

// Pause sends a pause request to every running job. No signal is sent to the
// API listener.
func (jm *JobManager) Pause() {
	log.Info("JobManager: requested pausing")
	close(jm.apiCancel)
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
		payload := errorPayload{Err: err.Error()}
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
