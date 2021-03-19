// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/plugins/reporters/targetsuccess"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"
	"github.com/facebookincubator/contest/plugins/targetmanagers/targetlist"
	"github.com/facebookincubator/contest/plugins/testfetchers/literal"
	testsIntegCommon "github.com/facebookincubator/contest/tests/integ/common"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/crash"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/fail"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/noop"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/noreturn"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/slowecho"
)

// Integration tests for the JobManager use an in-memory storage layer, which
// is a global singleton shared by all instances of JobManager. Therefore,
// JobManger tests cannot run in parallel with anything else in the same package
// that uses in-memory storage.
type CommandType string

const (
	StartJob CommandType = "start"
	StopJob  CommandType = "stop"
	Status   CommandType = "status"
	List     CommandType = "list"
)

var ctx = logrusctx.NewContext(logger.LevelDebug)

type command struct {
	commandType   CommandType
	jobID         types.JobID
	jobDescriptor string
	// List arguments
	states []job.State
	tags   []string
}

// TestListener implements a dummy api.Listener interface for testing purposes
type TestListener struct {
	// commandCh is an input channel to the Serve() function of the TestListener
	// which controls the type of operation that should be triggered towards the
	// JobManager
	commandCh <-chan command
	// responseCh is an input channel to the integrations tests where the
	// dummy TestListener forwards the responses coming from the API layer
	responseCh chan<- api.Response
	// errorCh is an input channel to the integration tests where the
	// dummy TestListener forwards errors coming from the API layer
	errorCh chan<- error
}

// Serve implements the main logic of a dummy listener which talks to the API
// layer to trigger actions in the JobManager
func (tl *TestListener) Serve(ctx xcontext.Context, contestApi *api.API) error {
	ctx.Debugf("Serving mock listener")
	for {
		ctx.Debugf("select")
		select {
		case command := <-tl.commandCh:
			ctx.Debugf("received command: %#+v", command)
			switch command.commandType {
			case StartJob:
				resp, err := contestApi.Start(ctx, "IntegrationTest", command.jobDescriptor)
				if err != nil {
					tl.errorCh <- err
				}
				tl.responseCh <- resp
			case StopJob:
				resp, err := contestApi.Stop(ctx, "IntegrationTest", command.jobID)
				if err != nil {
					tl.errorCh <- err
				}
				tl.responseCh <- resp
			case Status:
				resp, err := contestApi.Status(ctx, "IntegrationTest", command.jobID)
				if err != nil {
					tl.errorCh <- err
				}
				tl.responseCh <- resp
			case List:
				resp, err := contestApi.List(ctx, "IntegrationTest", command.states, command.tags)
				if err != nil {
					tl.errorCh <- err
				}
				tl.responseCh <- resp
			default:
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

func pollForEvent(eventManager frameworkevent.EmitterFetcher, ev event.Name, jobID types.JobID, timeout time.Duration) ([]frameworkevent.Event, error) {
	start := time.Now()
	for {
		select {
		case <-time.After(10 * time.Millisecond):
			queryFields := []frameworkevent.QueryField{
				frameworkevent.QueryJobID(jobID),
				frameworkevent.QueryEventName(ev),
			}
			ev, err := eventManager.Fetch(ctx, queryFields...)
			if err != nil {
				return nil, err
			}
			if len(ev) != 0 {
				return ev, nil
			}
			if time.Since(start) > timeout {
				return ev, fmt.Errorf("timeout")
			}
		}
	}
}

type TestJobManagerSuite struct {
	suite.Suite

	// storage is the storage engine initially configured by the upper level TestSuite,
	// which either configures a memory or a rdbms storage backend.
	storage storage.Storage

	// txStorage storage is initialized from storage at the beginning of each test. If
	// the backend supports transactions, txStorage runs within a transaction. At the end
	// of the job txStorage is finalized: it's either committed or rolled back, depending
	// what the backend supports
	txStorage storage.Storage

	jm *jobmanager.JobManager

	pluginRegistry *pluginregistry.PluginRegistry

	jsm          storage.JobStorageManager
	eventManager frameworkevent.EmitterFetcher
	targetLocker target.Locker

	// commandCh is the counterpart of the commandCh in the Listener
	commandCh chan command
	// responseCh is the counterpart of the responseCh in the Listener
	responseCh chan api.Response
	// errorCh is the counterpart of the errorCh in the Listener
	errorCh chan error
	// jobManagerCh is a control channel used to signal the termination of JobManager
	jobManagerCh chan struct{}
	// sigs is an input channel to the JobManager which carries UNIX signals
	// that could trigger pause or termination of the JobManager itself.
	sigs chan os.Signal
}

func (suite *TestJobManagerSuite) startJobManager() {
	go func() {
		_ = suite.jm.Start(ctx, suite.sigs)
		close(suite.jobManagerCh)
	}()
}

func (suite *TestJobManagerSuite) startJob(jobDescriptor string) (types.JobID, error) {
	var resp api.Response
	start := command{commandType: StartJob, jobDescriptor: jobDescriptor}
	suite.commandCh <- start
	select {
	case resp = <-suite.responseCh:
		if resp.Err != nil {
			return types.JobID(0), resp.Err
		}
	case <-time.After(2 * time.Second):
		return types.JobID(0), fmt.Errorf("Listener response should come within the timeout")
	}
	jobID := resp.Data.(api.ResponseDataStart).JobID
	return jobID, nil
}

func (suite *TestJobManagerSuite) stopJob(jobID types.JobID) error {
	var resp api.Response
	stop := command{commandType: StopJob, jobID: jobID}
	suite.commandCh <- stop
	select {
	case resp = <-suite.responseCh:
		if resp.Err != nil {
			return resp.Err
		}
	case <-time.After(2 * time.Second):
		return fmt.Errorf("Listener response should come within the timeout")
	}
	return nil
}

func (suite *TestJobManagerSuite) jobStatus(jobID types.JobID) (*job.Status, error) {
	suite.commandCh <- command{commandType: Status, jobID: jobID}
	var resp api.Response
	select {
	case resp = <-suite.responseCh:
		if resp.Err != nil {
			return nil, resp.Err
		}
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("Listener response should come within the timeout")
	}
	return resp.Data.(api.ResponseDataStatus).Status, nil
}

func (suite *TestJobManagerSuite) listJobs(states []job.State, tags []string) ([]types.JobID, error) {
	suite.commandCh <- command{commandType: List, states: states, tags: tags}
	var resp api.Response
	select {
	case resp = <-suite.responseCh:
		if resp.Err != nil {
			return nil, resp.Err
		}
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("Listener response should come within the timeout")
	}
	return resp.Data.(api.ResponseDataList).JobIDs, nil
}

func (suite *TestJobManagerSuite) SetupTest() {

	jsm := storage.NewJobStorageManager()
	eventManager := storage.NewFrameworkEventEmitterFetcher()

	suite.jsm = jsm
	suite.eventManager = eventManager

	pr := pluginregistry.NewPluginRegistry(ctx)
	pr.RegisterTargetManager(targetlist.Name, targetlist.New)
	pr.RegisterTestFetcher(literal.Name, literal.New)
	pr.RegisterReporter(targetsuccess.Name, targetsuccess.New)
	pr.RegisterTestStep(noop.Name, noop.New, noop.Events)
	pr.RegisterTestStep(fail.Name, fail.New, fail.Events)
	pr.RegisterTestStep(crash.Name, crash.New, crash.Events)
	pr.RegisterTestStep(noreturn.Name, noreturn.New, noreturn.Events)
	pr.RegisterTestStep(slowecho.Name, slowecho.New, slowecho.Events)
	suite.pluginRegistry = pr

	suite.txStorage = testsIntegCommon.InitStorage(suite.storage)
	require.NoError(suite.T(), storage.SetStorage(suite.txStorage))
	require.NoError(suite.T(), storage.SetAsyncStorage(suite.txStorage))

	suite.targetLocker = inmemory.New()
	target.SetLocker(suite.targetLocker)

	suite.initJobManager("")
}

func (suite *TestJobManagerSuite) initJobManager(instanceTag string) {
	suite.commandCh = make(chan command)
	suite.responseCh = make(chan api.Response)
	suite.errorCh = make(chan error)
	testListener := TestListener{commandCh: suite.commandCh, responseCh: suite.responseCh, errorCh: suite.errorCh}
	suite.jobManagerCh = make(chan struct{})
	var opts []jobmanager.Option
	if instanceTag != "" {
		opts = append(opts, jobmanager.OptionInstanceTag(instanceTag))
	}
	jm, err := jobmanager.New(&testListener, suite.pluginRegistry, opts...)
	require.NoError(suite.T(), err)
	suite.jm = jm
	suite.sigs = make(chan os.Signal)
}

func (suite *TestJobManagerSuite) stopJobManager() {
	// Signal cancellation to the JobManager, which in turn will
	// propagate cancellation signal to Serve method of the listener.
	// JobManager.Start() will return and close jobManagerCh.
	select {
	case suite.sigs <- syscall.SIGINT:
	case <-time.After(1 * time.Second):
		// Some tests do cancel the JobManager, so if is not respondive to SIGINT, it
		// might not be an issue. Regardless of where the cancellation signal originated,
		// JobManager.Start() should return and close jobManagerCh.
	}

	select {
	case <-suite.jobManagerCh:
	case <-time.After(2 * time.Second):
		suite.T().Errorf("JobManager should return within the timeout")
	}
}

func (suite *TestJobManagerSuite) TearDownTest() {
	suite.stopJobManager()
	testsIntegCommon.FinalizeStorage(suite.txStorage)
	storage.SetStorage(suite.storage)
	storage.SetAsyncStorage(suite.storage)
}

func (suite *TestJobManagerSuite) testExit(
	sig syscall.Signal,
	expectedEvent event.Name,
	exitTimeout time.Duration,
) types.JobID {

	jobID, err := suite.startJob(jobDescriptorSlowEcho)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobStarted when the Job is started
	ev, err := pollForEvent(suite.eventManager, job.EventJobStarted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
	time.Sleep(100 * time.Millisecond)

	// Signal cancellation to the JobManager, which in turn will
	// propagate cancellation signal to Serve method of the listener.
	// JobManager.Start() will return and close jobManagerCh.
	select {
	case suite.sigs <- sig:
	case <-time.After(1 * time.Second):
		suite.T().Fatalf("unable to push signal %s", sig)
	}

	select {
	case <-suite.jobManagerCh:
	case <-time.After(exitTimeout):
		suite.T().Errorf("JobManager should return within the timeout")
	}

	// JobManager will emit a paused or cancelled event when the job completes
	ev, err = suite.eventManager.Fetch(ctx,
		frameworkevent.QueryJobID(jobID),
		frameworkevent.QueryEventName(expectedEvent),
	)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev), expectedEvent)

	return jobID
}

func (suite *TestJobManagerSuite) TestPauseAndExit() {
	suite.startJobManager()

	suite.testExit(syscall.SIGINT, job.EventJobPaused, time.Second)
}

func (suite *TestJobManagerSuite) TestJobManagerJobStartSingle() {
	suite.startJobManager()

	jobID, err := suite.startJob(jobDescriptorNoop)
	require.NoError(suite.T(), err)

	_, err = suite.jsm.GetJobRequest(ctx, types.JobID(jobID))
	require.NoError(suite.T(), err)

	r, err := suite.jsm.GetJobRequest(ctx, types.JobID(jobID + 1))
	require.Error(suite.T(), err)
	require.NotEqual(suite.T(), nil, r)

	// JobManager will emit an EventJobStarted when the Job is started
	ev, err := pollForEvent(suite.eventManager, job.EventJobStarted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// JobManager will emit an EventJobCompleted when the Job completes
	ev, err = pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
}

func (suite *TestJobManagerSuite) TestJobManagerJobReport() {
	suite.startJobManager()

	jobID, err := suite.startJob(jobDescriptorNoop)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobCompleted when the Job completes
	ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// A Report must be persisted for the Job
	jobReport, err := suite.jsm.GetJobReport(ctx, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))

	// Any other Job should not have a Job report, but fetching the
	// report should not error out
	jobReport, err = suite.jsm.GetJobReport(ctx, types.JobID(2))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), &job.JobReport{JobID: 2}, jobReport)
}

func (suite *TestJobManagerSuite) TestJobManagerJobCancellation() {
	suite.startJobManager()

	jobID, err := suite.startJob(jobDescriptorSlowEcho)
	require.NoError(suite.T(), err)

	// Wait EventJobStarted event. This is necessary so that we can later issue a
	// Stop command for a Job that we know is already running.
	ev, err := pollForEvent(suite.eventManager, job.EventJobStarted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// Send Stop command to the job
	err = suite.stopJob(jobID)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobCancelling as soon as the cancellation signal
	// is asserted
	ev, err = pollForEvent(suite.eventManager, job.EventJobCancelling, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// JobManager will emit an EventJobCancelled event when the Job has completed
	// cancellation successfully (completing cancellation successfully means that
	// the TestRunner returns within the timeout and that TargetManage.Release()
	// all targets)
	ev, err = pollForEvent(suite.eventManager, job.EventJobCancelled, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
}

func (suite *TestJobManagerSuite) TestJobManagerJobNotSuccessful() {
	suite.startJobManager()

	jobID, err := suite.startJob(jobDescriptorFailure)
	require.NoError(suite.T(), err)

	// If the Job completes, but the result of the reporting phase indicates a failure,
	// an EventJobCompleted is emitted and the Report will indicate that the Job was unsuccessful
	ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	jobReport, err := suite.jsm.GetJobReport(ctx, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))
}

func (suite *TestJobManagerSuite) TestJobManagerJobFailure() {
	suite.startJobManager()

	jobID, err := suite.startJob(jobDescriptorFailure)
	require.NoError(suite.T(), err)

	// If the Job completes, but the result of the reporting phase indicates a failure,
	// an EventJobCompleted is emitted and the Report will indicate that the Job was unsuccessful
	ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	jobReport, err := suite.jsm.GetJobReport(ctx, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))
}

func (suite *TestJobManagerSuite) TestJobManagerJobCrash() {
	suite.startJobManager()

	jobID, err := suite.startJob(jobDescriptorCrash)
	require.NoError(suite.T(), err)
	// If the Job does not complete and returns an error instead, an EventJobFailed
	// is emitted. The report will indicate that the job was unsuccessful, and
	// the report calculate by the plugin will be nil
	ev, err := pollForEvent(suite.eventManager, job.EventJobFailed, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
	require.Contains(suite.T(), string(*ev[0].Payload), "TestStep crashed")
	jobReport, err := suite.jsm.GetJobReport(ctx, types.JobID(jobID))

	require.NoError(suite.T(), err)
	// no reports are expected if the job crashes
	require.Equal(suite.T(), &job.JobReport{JobID: jobID}, jobReport)
}

func (suite *TestJobManagerSuite) TestJobManagerJobCancellationFailure() {

	config.TestRunnerShutdownTimeout = 1 * time.Second
	config.TestRunnerStepShutdownTimeout = 1 * time.Second

	suite.startJobManager()

	jobID, err := suite.startJob(jobDescriptorHang)
	require.NoError(suite.T(), err)

	// Wait EventJobStarted event. This is necessary so that we can later issue a
	// Stop command for a Job that we know is already running.
	ev, err := pollForEvent(suite.eventManager, job.EventJobStarted, types.JobID(jobID), 5*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	err = suite.stopJob(jobID)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobCancelling as soon as the cancellation signal
	// is asserted
	ev, err = pollForEvent(suite.eventManager, job.EventJobCancelling, types.JobID(jobID), 5*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// JobManager will emit an EventJobCancelled event when the Job has
	// completed cancellation successfully (completing cancellation successfully
	// means that the TestRunner returns within the timeout and that
	// TargetManage.Release() all targets)
	ev, err = pollForEvent(suite.eventManager, job.EventJobCancelling, types.JobID(jobID), 5*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
}

func (suite *TestJobManagerSuite) TestTestStepNoLabel() {
	suite.startJobManager()

	jobId, err := suite.startJob(jobDescriptorNoLabel)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.True(suite.T(), errors.As(err, &pluginregistry.ErrStepLabelIsMandatory{}))
	require.Contains(suite.T(), err.Error(), "step has no label")
}

func (suite *TestJobManagerSuite) TestTestStepLabelDuplication() {
	suite.startJobManager()

	jobId, err := suite.startJob(jobDescriptorLabelDuplication)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), "found duplicated labels")
}

func (suite *TestJobManagerSuite) TestTestStepNull() {
	suite.startJobManager()

	jobId, err := suite.startJob(jobDescriptorNullStep)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), "test step description is null")
}

func (suite *TestJobManagerSuite) TestTestNull() {
	suite.startJobManager()

	jobId, err := suite.startJob(jobDescriptorNullTest)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), "test description is null")
}

func (suite *TestJobManagerSuite) TestBadTag() {
	suite.startJobManager()

	jobId, err := suite.startJob(jobDescriptorBadTag)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), `"a bad one" is not a valid tag`)
}

func (suite *TestJobManagerSuite) TestInternalTag() {
	suite.startJobManager()

	jobId, err := suite.startJob(jobDescriptorInternalTag)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), `"_foo" is an internal tag`)
}

func (suite *TestJobManagerSuite) TestDuplicateTag() {
	suite.startJobManager()

	jobId, err := suite.startJob(jobDescriptorDuplicateTag)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), `duplicate tag "qwe"`)
}

func (suite *TestJobManagerSuite) TestJobManagerDifferentInstances() {
	// Run a job in one instance.
	var err error
	var jobID types.JobID
	{
		suite.initJobManager("_A")
		suite.startJobManager()

		jobID, err = suite.startJob(jobDescriptorNoop)
		require.NoError(suite.T(), err)

		_, err = suite.jsm.GetJobRequest(ctx, types.JobID(jobID))
		require.NoError(suite.T(), err)

		ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
		require.NoError(suite.T(), err)
		require.Equal(suite.T(), 1, len(ev))

		// Make sure we can list the job and get its status.
		jobIDs, err := suite.listJobs(nil, nil)
		require.NoError(suite.T(), err)
		require.Equal(suite.T(), []types.JobID{jobID}, jobIDs)
		jobStatus, err := suite.jobStatus(jobID)
		require.NoError(suite.T(), err)
		require.NotNil(suite.T(), jobStatus)

		suite.stopJobManager()
	}

	// Make sure another instance doesn't see the job.
	{
		suite.initJobManager("_B")
		suite.startJobManager()
		jobIDs, err := suite.listJobs(nil, nil)
		require.NoError(suite.T(), err)
		require.Empty(suite.T(), jobIDs)
		jobStatus, err := suite.jobStatus(jobID)
		require.Error(suite.T(), err)
		require.Contains(suite.T(), err.Error(), "different instance")
		require.Nil(suite.T(), jobStatus)
	}
}

func (suite *TestJobManagerSuite) TestJobListing() {
	suite.startJobManager()

	// There are no jobs to begin with.
	jobIDs, err := suite.listJobs(nil, nil)
	require.NoError(suite.T(), err)
	require.Empty(suite.T(), jobIDs)

	// Run two jobs, one successful one not.
	jobID1, err := suite.startJob(jobDescriptorNoop2)
	require.NoError(suite.T(), err)
	ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID1, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
	jobID2, err := suite.startJob(jobDescriptorCrash)
	require.NoError(suite.T(), err)
	ev, err = pollForEvent(suite.eventManager, job.EventJobFailed, jobID2, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// No filters
	jobIDs, err = suite.listJobs(nil, nil)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1, jobID2}, jobIDs)

	// Filter by state - any state should match
	jobIDs, err = suite.listJobs([]job.State{job.JobStateCompleted}, nil)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1}, jobIDs)
	jobIDs, err = suite.listJobs([]job.State{job.JobStateFailed}, nil)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID2}, jobIDs)
	jobIDs, err = suite.listJobs([]job.State{job.JobStateCompleted, job.JobStateCancelled, job.JobStateFailed}, nil)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1, jobID2}, jobIDs)

	// Filter by tags - all that specified tags must be present.
	jobIDs, err = suite.listJobs(nil, []string{"integration_testing"})
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1, jobID2}, jobIDs)
	jobIDs, err = suite.listJobs(nil, []string{"foo", "integration_testing"})
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1}, jobIDs)

	// Filter by both tags and state.
	jobIDs, err = suite.listJobs([]job.State{job.JobStateCompleted}, []string{"integration_testing"})
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1}, jobIDs)
}
