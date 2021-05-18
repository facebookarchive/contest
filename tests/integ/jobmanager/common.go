// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/logging"
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
	testsCommon "github.com/facebookincubator/contest/tests/common"
	"github.com/facebookincubator/contest/tests/common/goroutine_leak_check"
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

type command struct {
	commandType   CommandType
	jobID         types.JobID
	jobDescriptor string
	// List arguments
	jobQuery *storage.JobQuery
}

const fakeJobID types.JobID = 1234567

// TestListener implements a dummy api.Listener interface for testing purposes
type TestListener struct {
	// API instance this listener is serving
	api *api.API
	// commandCh is an input channel to the Serve() function of the TestListener
	// which controls the type of operation that should be triggered towards the
	// JobManager
	commandCh chan command
	// responseCh is an input channel to the integrations tests where the
	// dummy TestListener forwards the responses coming from the API layer
	responseCh chan api.Response
	// errorCh is an input channel to the integration tests where the
	// dummy TestListener forwards errors coming from the API layer
	errorCh chan error
}

// Serve implements the main logic of a dummy listener which talks to the API
// layer to trigger actions in the JobManager
func (tl *TestListener) Serve(ctx xcontext.Context, contestApi *api.API) error {
	ctx.Debugf("Serving mock listener")
	tl.api = contestApi
	for {
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
				resp, err := contestApi.List(ctx, "IntegrationTest", command.jobQuery)
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
			ev, err := eventManager.Fetch(xcontext.Background(), queryFields...)
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

	// TODO(rojer): use mock clock here.
	// clock *clock.Mock
	clock clock.Clock

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

	// API listener
	listener *TestListener

	// jobManagerCh is a control channel used to signal the termination of JobManager
	jobManagerCh chan struct{}
	// ctx is an input context to the JobManager which can be used to pause or cancel JobManager
	jmCtx             xcontext.Context
	jmPause, jmCancel func()
}

func (suite *TestJobManagerSuite) startJobManager(resumeJobs bool) {
	go func() {
		_ = suite.jm.Run(suite.jmCtx, resumeJobs)
		close(suite.jobManagerCh)
	}()
}

func (suite *TestJobManagerSuite) startJob(jobDescriptor string) (types.JobID, error) {
	var resp api.Response
	start := command{commandType: StartJob, jobDescriptor: jobDescriptor}
	suite.listener.commandCh <- start
	select {
	case resp = <-suite.listener.responseCh:
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
	suite.listener.commandCh <- stop
	select {
	case resp = <-suite.listener.responseCh:
		if resp.Err != nil {
			return resp.Err
		}
	case <-time.After(2 * time.Second):
		return fmt.Errorf("Listener response should come within the timeout")
	}
	return nil
}

func (suite *TestJobManagerSuite) jobStatus(jobID types.JobID) (*job.Status, error) {
	suite.listener.commandCh <- command{commandType: Status, jobID: jobID}
	var resp api.Response
	select {
	case resp = <-suite.listener.responseCh:
		if resp.Err != nil {
			return nil, resp.Err
		}
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("Listener response should come within the timeout")
	}
	return resp.Data.(api.ResponseDataStatus).Status, nil
}

func (suite *TestJobManagerSuite) listJobs(states []job.State, tags []string, serverID string) ([]types.JobID, error) {
	var fields []storage.JobQueryField
	if len(states) > 0 {
		fields = append(fields, storage.QueryJobStates(states...))
	}
	if len(tags) > 0 {
		fields = append(fields, storage.QueryJobTags(tags...))
	}
	if len(serverID) > 0 {
		fields = append(fields, storage.QueryJobServerID(serverID))
	}
	jobQuery, err := storage.BuildJobQuery(fields...)
	if err != nil {
		return nil, err
	}
	suite.listener.commandCh <- command{commandType: List, jobQuery: jobQuery}
	var resp api.Response
	select {
	case resp = <-suite.listener.responseCh:
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

	// TODO(rojer): Use mock clock to speed up the tests.
	// suite.clock = clock.NewMock()
	// suite.clock.Add(1 * time.Hour) // start at 01:00
	suite.clock = clock.New()
	slowecho.Clock = suite.clock

	suite.jsm = jsm
	suite.eventManager = eventManager

	pr := pluginregistry.NewPluginRegistry(xcontext.Background())
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

	suite.targetLocker = inmemory.New(suite.clock)
	target.SetLocker(suite.targetLocker)

	suite.initJobManager("")
}

func (suite *TestJobManagerSuite) initJobManager(instanceTag string) {
	suite.listener = &TestListener{commandCh: make(chan command), responseCh: make(chan api.Response), errorCh: make(chan error)}
	suite.jobManagerCh = make(chan struct{})
	opts := []jobmanager.Option{jobmanager.OptionClock(suite.clock)}
	if instanceTag != "" {
		opts = append(opts, jobmanager.OptionInstanceTag(instanceTag))
	}
	jm, err := jobmanager.New(suite.listener, suite.pluginRegistry, opts...)
	require.NoError(suite.T(), err)
	suite.jm = jm
	suite.jmCtx, suite.jmCancel = xcontext.WithCancel(logrusctx.NewContext(logger.LevelDebug, logging.DefaultOptions()...))
	suite.jmCtx, suite.jmPause = xcontext.WithNotify(suite.jmCtx, xcontext.ErrPaused)
}

func (suite *TestJobManagerSuite) BeforeTest(suiteName, testName string) {
	suite.jmCtx.Infof("=== Running %s/%s", suiteName, testName)
}

func (suite *TestJobManagerSuite) stopJobManager() {
	// Signal cancellation to the JobManager, which in turn will
	// propagate cancellation signal to Serve method of the listener.
	// JobManager.Start() will return and close jobManagerCh.
	suite.jmCancel()

	select {
	case <-suite.jobManagerCh:
	case <-time.After(2 * time.Second):
		suite.T().Errorf("JobManager should return within the timeout")
	}
}

func (suite *TestJobManagerSuite) TearDownTest() {
	suite.stopJobManager()
	testsIntegCommon.FinalizeStorage(suite.txStorage)
	storage.SetStorage(nil)
	storage.SetAsyncStorage(nil)
	target.SetLocker(nil)
	slowecho.Clock = nil
}

func (suite *TestJobManagerSuite) TearDownSuite() {
	if err := suite.storage.Close(); err != nil {
		panic("Failed to close storage")
	}
	time.Sleep(20 * time.Millisecond)
	if err := goroutine_leak_check.CheckLeakedGoRoutines(); err != nil {
		panic(fmt.Sprintf("%s", err))
	}
}

func (suite *TestJobManagerSuite) verifyTargetLockStatus(targetIDs []string, expectLocked bool) {
	var targetsToLock, targetsToUnlock []*target.Target
	for _, id := range targetIDs {
		targetsToLock = append(targetsToLock, &target.Target{ID: id})
	}
	lockedTargets, err := suite.targetLocker.TryLock(suite.jmCtx, fakeJobID, 1*time.Second, targetsToLock, uint(len(targetsToLock)))
	require.NoError(suite.T(), err)
	// Immediately unlock any targets we may have locked, this was only a test.
	if len(lockedTargets) > 0 {
		for _, id := range lockedTargets {
			targetsToUnlock = append(targetsToUnlock, &target.Target{ID: id})
		}
		require.NoError(suite.T(), suite.targetLocker.Unlock(suite.jmCtx, fakeJobID, targetsToUnlock))
	}
	if expectLocked {
		// We expect all the targets to be locked, so we shouldn't be able to lock any.
		require.Empty(suite.T(), lockedTargets, "expected targets to be locked but some were not")
	} else {
		// We expect targets to be unlocked, so we must have successfully locked all of them.
		var badTargets []string
		for _, id := range targetIDs {
			found := false
			for _, lid := range lockedTargets {
				if id == lid {
					found = true
					break
				}
			}
			if !found {
				badTargets = append(badTargets, id)
			}
		}
		require.Empty(suite.T(), badTargets, "targets are locked but shouldn't be")
	}
}

func (suite *TestJobManagerSuite) testExit(
	sig func(),
	expectedEvent event.Name,
	exitTimeout time.Duration,
) types.JobID {

	suite.startJobManager(false /* resumeJobs */)

	jobID, err := suite.startJob(jobDescriptorSlowEcho)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobStarted when the Job is started
	ev, err := pollForEvent(suite.eventManager, job.EventJobStarted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
	time.Sleep(100 * time.Millisecond)

	// Signal pause or cancellation to the manager which in turn will
	// propagate cancellation signal to Serve method of the listener
	// and the running job.
	if sig != nil {
		sig()
	}

	select {
	case <-suite.jobManagerCh:
		suite.jmCtx.Infof("jm finished")
	case <-time.After(exitTimeout):
		suite.T().Errorf("JobManager should return within the timeout")
	}

	// JobManager will emit a paused or cancelled event when the job completes
	ev, err = suite.eventManager.Fetch(
		suite.jmCtx,
		frameworkevent.QueryJobID(jobID),
		frameworkevent.QueryEventName(expectedEvent),
	)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev), expectedEvent)

	return jobID
}

func (suite *TestJobManagerSuite) TestCancelAndExit() {
	suite.testExit(suite.jmCancel, job.EventJobCancelled, 2*time.Second)
	// Targets must be released when job is canceled.
	suite.verifyTargetLockStatus([]string{"id1", "id2"}, false)
}

func (suite *TestJobManagerSuite) TestPauseAndExit() {
	suite.testExit(suite.jmPause, job.EventJobPaused, 3*time.Second)
	// Targets should remain locked.
	suite.verifyTargetLockStatus([]string{"id1", "id2"}, true)
}

func (suite *TestJobManagerSuite) testPauseAndResume(
	pauseAfter time.Duration, lockedAfterPause bool, mutator func(jobID types.JobID),
	finalState event.Name, lockedAfterResume bool) {
	var jobID types.JobID

	// Run the job and pause.
	{
		var err error

		suite.startJobManager(true /* resumeJobs */)

		jobID, err = suite.startJob(jobDescriptorSlowEcho2)
		require.NoError(suite.T(), err)

		// JobManager will emit an EventJobStarted when the Job is started
		ev, err := pollForEvent(suite.eventManager, job.EventJobStarted, jobID, 1*time.Second)
		require.NoError(suite.T(), err)
		require.Equal(suite.T(), 1, len(ev))

		// Wait for second run to begin
		time.Sleep(pauseAfter)

		// Signal pause to the manager.
		suite.jmCtx.Infof("-> pausing")
		suite.jmPause()

		select {
		case <-suite.jobManagerCh:
		case <-time.After(3 * time.Second):
			suite.T().Errorf("JobManager should return within the timeout")
		}

		ev, err = suite.eventManager.Fetch(
			suite.jmCtx,
			frameworkevent.QueryJobID(jobID),
			frameworkevent.QueryEventName(job.EventJobPaused),
		)
		require.NoError(suite.T(), err)
		require.Equal(suite.T(), 1, len(ev))
	}

	// Verify target lock state after the job has been paused.
	suite.verifyTargetLockStatus([]string{"id1", "id2"}, lockedAfterPause)

	// If there is a state mutator to run, do it now.
	if mutator != nil {
		mutator(jobID)
	}

	// Create a new JobManager instance.
	suite.initJobManager("")

	// Resume and run the job to completion.
	{
		suite.startJobManager(true /* resumeJobs */)
		ev, err := pollForEvent(suite.eventManager, finalState, jobID, 5*time.Second)
		require.NoError(suite.T(), err)
		require.Equal(suite.T(), 1, len(ev))
		suite.jmCancel()
		select {
		case <-suite.jobManagerCh:
		case <-time.After(3 * time.Second):
			suite.T().Errorf("JobManager should return within the timeout")
		}
	}

	// Verify final target lock state.
	suite.verifyTargetLockStatus([]string{"id1", "id2"}, lockedAfterResume)

	if finalState == job.EventJobCompleted {
		// Verify emitted events. Despite pausing ad different stages this should look perfectly normal.
		require.Equal(suite.T(), strings.Replace(`
{[JOBID 1 IntegrationTest: resume ][Target{ID: "id1"} TargetAcquired]}
{[JOBID 1 IntegrationTest: resume Step 1][Target{ID: "id1"} TargetIn]}
{[JOBID 1 IntegrationTest: resume Step 1][Target{ID: "id1"} TargetOut]}
{[JOBID 1 IntegrationTest: resume Step 2][Target{ID: "id1"} TargetIn]}
{[JOBID 1 IntegrationTest: resume Step 2][Target{ID: "id1"} TargetOut]}
{[JOBID 1 IntegrationTest: resume ][Target{ID: "id1"} TargetReleased]}
{[JOBID 2 IntegrationTest: resume ][Target{ID: "id1"} TargetAcquired]}
{[JOBID 2 IntegrationTest: resume Step 1][Target{ID: "id1"} TargetIn]}
{[JOBID 2 IntegrationTest: resume Step 1][Target{ID: "id1"} TargetOut]}
{[JOBID 2 IntegrationTest: resume Step 2][Target{ID: "id1"} TargetIn]}
{[JOBID 2 IntegrationTest: resume Step 2][Target{ID: "id1"} TargetOut]}
{[JOBID 2 IntegrationTest: resume ][Target{ID: "id1"} TargetReleased]}
`, "JOBID", fmt.Sprintf("%d", jobID), -1),
			suite.getTargetEvents("IntegrationTest: resume", "id1"))
	}
}

func (suite *TestJobManagerSuite) TestPauseAndResumeDuringRun1() {
	// When paused during a run, targets should remain locked.
	suite.testPauseAndResume(250*time.Millisecond, true, nil, job.EventJobCompleted, false)
}

func (suite *TestJobManagerSuite) TestPauseAndResumeBetweenRuns() {
	// Pause between runs. Targets should not be locked in this case.
	suite.testPauseAndResume(750*time.Millisecond, false, func(jobID types.JobID) {
		// Report for the completed run must be persisted already.
		jobReport, err := suite.jsm.GetJobReport(suite.jmCtx, jobID)
		require.NoError(suite.T(), err)
		require.Equal(suite.T(), 1, len(jobReport.RunReports))
		require.Equal(suite.T(), 0, len(jobReport.FinalReports))
	}, job.EventJobCompleted, false)
}

func (suite *TestJobManagerSuite) TestPauseAndResumeDuringRun2() {
	// When paused during a run, targets should remain locked.
	suite.testPauseAndResume(1250*time.Millisecond, true, nil, job.EventJobCompleted, false)
}

func (suite *TestJobManagerSuite) TestPauseAndFailToResume() {
	v := job.CurrentPauseEventPayloadVersion
	defer func() { job.CurrentPauseEventPayloadVersion = v }()
	suite.testPauseAndResume(1250*time.Millisecond, true, func(_ types.JobID) {
		job.CurrentPauseEventPayloadVersion = -1 // Resume will fail due to incompatible version
	}, job.EventJobFailed,
		// Targets remain locked because we were unable to deserialize the state.
		// Unfortunately, there is nothing we can do.
		true)
}

func (suite *TestJobManagerSuite) getTargetEvents(testName, targetID string) string {
	return suite.getEvents(testName, &targetID, nil)
}

func (suite *TestJobManagerSuite) getEvents(testName string, targetID, stepLabel *string) string {
	return testsCommon.GetTestEventsAsString(suite.jmCtx, suite.txStorage, testName, targetID, stepLabel)
}

func (suite *TestJobManagerSuite) TestJobManagerJobStartSingle() {
	suite.startJobManager(false /* resumeJobs */)

	jobID, err := suite.startJob(jobDescriptorNoop)
	require.NoError(suite.T(), err)

	_, err = suite.jsm.GetJobRequest(suite.jmCtx, jobID)
	require.NoError(suite.T(), err)

	r, err := suite.jsm.GetJobRequest(suite.jmCtx, jobID+1)
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
	suite.startJobManager(false /* resumeJobs */)

	jobID, err := suite.startJob(jobDescriptorNoop)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobCompleted when the Job completes
	ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// A Report must be persisted for the Job
	jobReport, err := suite.jsm.GetJobReport(suite.jmCtx, jobID)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))

	// Any other Job should not have a Job report, but fetching the
	// report should not error out
	jobReport, err = suite.jsm.GetJobReport(suite.jmCtx, types.JobID(2))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), &job.JobReport{JobID: 2}, jobReport)
}

func (suite *TestJobManagerSuite) TestJobManagerJobCancellation() {
	suite.startJobManager(false /* resumeJobs */)

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
	suite.startJobManager(false /* resumeJobs */)

	jobID, err := suite.startJob(jobDescriptorFailure)
	require.NoError(suite.T(), err)

	// If the Job completes, but the result of the reporting phase indicates a failure,
	// an EventJobCompleted is emitted and the Report will indicate that the Job was unsuccessful
	ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	jobReport, err := suite.jsm.GetJobReport(suite.jmCtx, jobID)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))
}

func (suite *TestJobManagerSuite) TestJobManagerJobFailure() {
	suite.startJobManager(false /* resumeJobs */)

	jobID, err := suite.startJob(jobDescriptorFailure)
	require.NoError(suite.T(), err)

	// If the Job completes, but the result of the reporting phase indicates a failure,
	// an EventJobCompleted is emitted and the Report will indicate that the Job was unsuccessful
	ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	jobReport, err := suite.jsm.GetJobReport(suite.jmCtx, jobID)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))
}

func (suite *TestJobManagerSuite) TestJobManagerJobCrash() {
	suite.startJobManager(false /* resumeJobs */)

	jobID, err := suite.startJob(jobDescriptorCrash)
	require.NoError(suite.T(), err)
	// If the Job does not complete and returns an error instead, an EventJobFailed
	// is emitted. The report will indicate that the job was unsuccessful, and
	// the report calculate by the plugin will be nil
	ev, err := pollForEvent(suite.eventManager, job.EventJobFailed, jobID, 1*time.Second)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
	require.Contains(suite.T(), string(*ev[0].Payload), "TestStep crashed")
	jobReport, err := suite.jsm.GetJobReport(suite.jmCtx, jobID)

	require.NoError(suite.T(), err)
	// no reports are expected if the job crashes
	require.Equal(suite.T(), &job.JobReport{JobID: jobID}, jobReport)
}

func (suite *TestJobManagerSuite) TestTestStepNoLabel() {
	suite.startJobManager(false /* resumeJobs */)

	jobId, err := suite.startJob(jobDescriptorNoLabel)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.True(suite.T(), errors.As(err, &pluginregistry.ErrStepLabelIsMandatory{}))
	require.Contains(suite.T(), err.Error(), "step has no label")
}

func (suite *TestJobManagerSuite) TestTestStepLabelDuplication() {
	suite.startJobManager(false /* resumeJobs */)

	jobId, err := suite.startJob(jobDescriptorLabelDuplication)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), "found duplicated labels")
}

func (suite *TestJobManagerSuite) TestTestStepNull() {
	suite.startJobManager(false /* resumeJobs */)

	jobId, err := suite.startJob(jobDescriptorNullStep)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), "test step description is null")
}

func (suite *TestJobManagerSuite) TestTestNull() {
	suite.startJobManager(false /* resumeJobs */)

	jobId, err := suite.startJob(jobDescriptorNullTest)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), "test description is null")
}

func (suite *TestJobManagerSuite) TestBadTag() {
	suite.startJobManager(false /* resumeJobs */)

	jobId, err := suite.startJob(jobDescriptorBadTag)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), `"a bad one" is not a valid tag`)
}

func (suite *TestJobManagerSuite) TestInternalTag() {
	suite.startJobManager(false /* resumeJobs */)

	jobId, err := suite.startJob(jobDescriptorInternalTag)
	require.Error(suite.T(), err)
	require.Equal(suite.T(), types.JobID(0), jobId)
	require.Contains(suite.T(), err.Error(), `"_foo" is an internal tag`)
}

func (suite *TestJobManagerSuite) TestDuplicateTag() {
	suite.startJobManager(false /* resumeJobs */)

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
		suite.startJobManager(false /* resumeJobs */)

		jobID, err = suite.startJob(jobDescriptorNoop)
		require.NoError(suite.T(), err)

		_, err = suite.jsm.GetJobRequest(suite.jmCtx, types.JobID(jobID))
		require.NoError(suite.T(), err)

		ev, err := pollForEvent(suite.eventManager, job.EventJobCompleted, jobID, 1*time.Second)
		require.NoError(suite.T(), err)
		require.Equal(suite.T(), 1, len(ev))

		// Make sure we can list the job and get its status.
		jobIDs, err := suite.listJobs(nil, nil, "")
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
		suite.startJobManager(false /* resumeJobs */)
		jobIDs, err := suite.listJobs(nil, nil, "")
		require.NoError(suite.T(), err)
		require.Empty(suite.T(), jobIDs)
		jobStatus, err := suite.jobStatus(jobID)
		require.Error(suite.T(), err)
		require.Contains(suite.T(), err.Error(), "different instance")
		require.Nil(suite.T(), jobStatus)
	}
}

func (suite *TestJobManagerSuite) TestJobListing() {
	suite.startJobManager(false /* resumeJobs */)

	// There are no jobs to begin with.
	jobIDs, err := suite.listJobs(nil, nil, "")
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
	jobIDs, err = suite.listJobs(nil, nil, "")
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1, jobID2}, jobIDs)

	// Filter by state - any state should match
	jobIDs, err = suite.listJobs([]job.State{job.JobStateCompleted}, nil, "")
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1}, jobIDs)
	jobIDs, err = suite.listJobs([]job.State{job.JobStateFailed}, nil, "")
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID2}, jobIDs)
	jobIDs, err = suite.listJobs([]job.State{job.JobStateCompleted, job.JobStateCancelled, job.JobStateFailed}, nil, "")
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1, jobID2}, jobIDs)

	// Filter by tags - all that specified tags must be present.
	jobIDs, err = suite.listJobs(nil, []string{"integration_testing"}, "")
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1, jobID2}, jobIDs)
	jobIDs, err = suite.listJobs(nil, []string{"foo", "integration_testing"}, "")
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1}, jobIDs)

	// Filter by both tags and state.
	jobIDs, err = suite.listJobs([]job.State{job.JobStateCompleted}, []string{"integration_testing"}, "")
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1}, jobIDs)

	// Filter by server ID.
	jobIDs, err = suite.listJobs(nil, nil, "foo")
	require.NoError(suite.T(), err)
	require.Empty(suite.T(), jobIDs)
	jobIDs, err = suite.listJobs(nil, nil, suite.listener.api.ServerID())
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), []types.JobID{jobID1, jobID2}, jobIDs)
}
