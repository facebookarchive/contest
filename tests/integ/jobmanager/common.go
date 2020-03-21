// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/jobmanager"

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/plugins/reporters/targetsuccess"
	"github.com/facebookincubator/contest/plugins/targetmanagers/targetlist"
	"github.com/facebookincubator/contest/plugins/testfetchers/literal"
	"github.com/facebookincubator/contest/plugins/teststeps/slowecho"
	"github.com/facebookincubator/contest/tests/integ/common"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/crash"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/fail"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/noop"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/noreturn"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Integration tests for the JobManager use an in-memory storage layer, which
// is a global singleton shared by all instances of JobManager. Therefore,
// JobManger tests cannot run in parallel with anything else in the same package
// that uses in-memory storage.
type CommandType string

var (
	StartJob CommandType = "start"
	StopJob  CommandType = "stop"
)

type command struct {
	commandType   CommandType
	jobID         types.JobID
	jobDescriptor string
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
func (tl *TestListener) Serve(cancel <-chan struct{}, contestApi *api.API) error {
	for {
		select {
		case command := <-tl.commandCh:
			if command.commandType == StartJob {
				resp, err := contestApi.Start("IntegrationTest", command.jobDescriptor)
				if err != nil {
					tl.errorCh <- err
				}
				tl.responseCh <- resp
			} else if command.commandType == StopJob {
				resp, err := contestApi.Stop("IntegrationTest", command.jobID)
				if err != nil {
					tl.errorCh <- err
				}
				tl.responseCh <- resp
			} else {
				panic(fmt.Sprintf("Command %v not supported", command))
			}
		case <-cancel:
			return nil
		}
	}
	return nil
}

func pollForEvent(eventManager frameworkevent.EmitterFetcher, ev event.Name, jobID types.JobID) ([]frameworkevent.Event, error) {
	var pollAttempt int
	for {
		select {
		case <-time.After(1 * time.Second):
			queryFields := []frameworkevent.QueryField{
				frameworkevent.QueryJobID(jobID),
				frameworkevent.QueryEventName(ev),
			}
			ev, err := eventManager.Fetch(queryFields...)
			if err != nil {
				return nil, err
			}
			if len(ev) != 0 {
				return ev, nil
			}
			pollAttempt += 1
			if pollAttempt == 5 {
				return ev, nil
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

	jobRequestManager job.RequestEmitterFetcher
	jobReportManager  job.ReportEmitterFetcher
	eventManager      frameworkevent.EmitterFetcher

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

func (suite *TestJobManagerSuite) SetupTest() {

	jobRequestManager := storage.NewJobRequestEmitterFetcher()
	jobReportManager := storage.NewJobReportEmitterFetcher()
	eventManager := storage.NewFrameworkEventEmitterFetcher()

	suite.jobRequestManager = jobRequestManager
	suite.jobReportManager = jobReportManager
	suite.eventManager = eventManager

	commandCh := make(chan command)
	suite.commandCh = commandCh
	responseCh := make(chan api.Response)
	suite.responseCh = responseCh
	errorCh := make(chan error)
	suite.errorCh = errorCh
	jobManagerCh := make(chan struct{})
	suite.jobManagerCh = jobManagerCh

	testListener := TestListener{commandCh: commandCh, responseCh: responseCh, errorCh: errorCh}

	logging.Disable()

	pluginRegistry := pluginregistry.NewPluginRegistry()
	pluginRegistry.RegisterTargetManager(targetlist.Name, targetlist.New)
	pluginRegistry.RegisterTestFetcher(literal.Name, literal.New)
	pluginRegistry.RegisterReporter(targetsuccess.Name, targetsuccess.New)
	pluginRegistry.RegisterTestStep(noop.Name, noop.New, noop.Events)
	pluginRegistry.RegisterTestStep(fail.Name, fail.New, fail.Events)
	pluginRegistry.RegisterTestStep(crash.Name, crash.New, crash.Events)
	pluginRegistry.RegisterTestStep(noreturn.Name, noreturn.New, noreturn.Events)
	pluginRegistry.RegisterTestStep(slowecho.Name, slowecho.New, slowecho.Events)

	jm, err := jobmanager.New(&testListener, pluginRegistry)
	require.NoError(suite.T(), err)

	suite.jm = jm
	sigs := make(chan os.Signal)
	suite.sigs = sigs

	suite.txStorage = common.InitStorage(suite.storage)
	storage.SetStorage(suite.txStorage)
}

func (suite *TestJobManagerSuite) TearDownTest() {

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
	common.FinalizeStorage(suite.txStorage)
	storage.SetStorage(suite.storage)
}

func (suite *TestJobManagerSuite) TestJobManagerJobStartSingle() {

	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	jobID, err := suite.startJob(jobDescriptorNoop)
	require.NoError(suite.T(), err)

	_, err = suite.jobRequestManager.Fetch(types.JobID(jobID))
	require.NoError(suite.T(), err)

	r, err := suite.jobRequestManager.Fetch(types.JobID(jobID + 1))
	require.Error(suite.T(), err)
	require.NotEqual(suite.T(), nil, r)

	// JobManager will emit an EventJobStarted when the Job is started
	ev, err := pollForEvent(suite.eventManager, jobmanager.EventJobStarted, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// JobManager will emit an EventJobCompleted when the Job completes
	ev, err = pollForEvent(suite.eventManager, jobmanager.EventJobCompleted, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

}

func (suite *TestJobManagerSuite) TestJobManagerJobReport() {
	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	jobID, err := suite.startJob(jobDescriptorNoop)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobCompleted when the Job completes
	ev, err := pollForEvent(suite.eventManager, jobmanager.EventJobCompleted, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// A Report must be persisted for the Job
	jobReport, err := suite.jobReportManager.Fetch(types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))

	// Any other Job should not have a Job report, but fetching the
	// report should not error out
	jobReport, err = suite.jobReportManager.Fetch(types.JobID(2))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), &job.JobReport{JobID: 2}, jobReport)
}

func (suite *TestJobManagerSuite) TestJobManagerJobCancellation() {

	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	jobID, err := suite.startJob(jobDescriptorSlowecho)
	require.NoError(suite.T(), err)

	// Wait EventJobStarted event. This is necessary so that we can later issue a
	// Stop command for a Job that we know is already running.
	ev, err := pollForEvent(suite.eventManager, jobmanager.EventJobStarted, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// Send Stop command to the job
	err = suite.stopJob(jobID)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobCancelling as soon as the cancellation signal
	// is asserted
	ev, err = pollForEvent(suite.eventManager, jobmanager.EventJobCancelling, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// JobManager will emit an EventJobCancelled event when the Job has completed
	// cancellation successfully (completing cancellation successfully means that
	// the TestRunner returns within the timeout and that TargetManage.Release()
	// all targets)
	ev, err = pollForEvent(suite.eventManager, jobmanager.EventJobCancelled, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
}

func (suite *TestJobManagerSuite) TestJobManagerJobNotSuccessful() {

	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	jobID, err := suite.startJob(jobDescriptorFailure)
	require.NoError(suite.T(), err)

	// If the Job completes, but the result of the reporting phase indicates a failure,
	// an EventJobCompleted is emitted and the Report will indicate that the Job was unsuccessful
	ev, err := pollForEvent(suite.eventManager, jobmanager.EventJobCompleted, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	jobReport, err := suite.jobReportManager.Fetch(types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))
}

func (suite *TestJobManagerSuite) TestJobManagerJobFailure() {

	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	jobID, err := suite.startJob(jobDescriptorFailure)
	require.NoError(suite.T(), err)

	// If the Job completes, but the result of the reporting phase indicates a failure,
	// an EventJobCompleted is emitted and the Report will indicate that the Job was unsuccessful
	ev, err := pollForEvent(suite.eventManager, jobmanager.EventJobCompleted, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	jobReport, err := suite.jobReportManager.Fetch(types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(jobReport.RunReports))
	require.Equal(suite.T(), 0, len(jobReport.FinalReports))
}

func (suite *TestJobManagerSuite) TestJobManagerJobCrash() {

	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()
	jobID, err := suite.startJob(jobDescriptorCrash)
	require.NoError(suite.T(), err)
	// If the Job does not complete and returns an error instead, an EventJobFailed
	// is emitted. The report will indicate that the job was unsuccessful, and
	// the report calculate by the plugin will be nil
	ev, err := pollForEvent(suite.eventManager, jobmanager.EventJobFailed, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))
	require.Equal(suite.T(), string(*ev[0].Payload), "{\"Err\":\"TestStep crashed\"}")
	jobReport, err := suite.jobReportManager.Fetch(types.JobID(jobID))

	require.NoError(suite.T(), err)
	// no reports are expected if the job crashes
	require.Equal(suite.T(), &job.JobReport{JobID: jobID}, jobReport)
}

func (suite *TestJobManagerSuite) TestJobManagerJobCancellationFailure() {

	config.TestRunnerShutdownTimeout = 1 * time.Second
	config.TestRunnerStepShutdownTimeout = 1 * time.Second

	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	jobID, err := suite.startJob(jobDescriptorHang)
	require.NoError(suite.T(), err)

	// Wait EventJobStarted event. This is necessary so that we can later issue a
	// Stop command for a Job that we know is already running.
	ev, err := pollForEvent(suite.eventManager, jobmanager.EventJobStarted, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	err = suite.stopJob(jobID)
	require.NoError(suite.T(), err)

	// JobManager will emit an EventJobCancelling as soon as the cancellation signal
	// is asserted
	ev, err = pollForEvent(suite.eventManager, jobmanager.EventJobCancelling, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

	// JobManager will emit an EventJobCancelled event when the Job has
	// completed cancellation successfully (completing cancellation successfully
	// means that the TestRunner returns within the timeout and that
	// TargetManage.Release() all targets)
	ev, err = pollForEvent(suite.eventManager, jobmanager.EventJobCancellationFailed, types.JobID(jobID))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(ev))

}

func (suite *TestJobManagerSuite) TestTestStepNoLabel() {
	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	_, err := suite.startJob(jobDescriptorNoLabel)
	require.Error(suite.T(), err)
	requireErrorType(suite.T(), err, pluginregistry.ErrStepLabelIsMandatory{})
}

func (suite *TestJobManagerSuite) TestTestStepLabelDuplication() {
	go func() {
		suite.jm.Start(suite.sigs)
		close(suite.jobManagerCh)
	}()

	_, err := suite.startJob(jobDescriptorLabelDuplication)
	require.Error(suite.T(), err)
}
