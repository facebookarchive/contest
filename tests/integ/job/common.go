// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/tests/integ/common"
)

var jobDescriptorFirst = `
  {
    "JobName": "FirstJob",
    "Runs": 1,
    "RunInterval": "5s",
    "Tags": ["integ", "tests", "foo"]
  }
`
var jobDescriptorSecond = `
  {
    "JobName": "SecondJob",
    "Runs": 1,
    "RunInterval": "5s",
    "Tags": ["integ"]
  }
`

var testDescs = `
[
  [
    {
      "name": "cmd",
      "label": "some cmd",
      "parameters": {}
    }
  ]
]
`

type JobSuite struct {
	suite.Suite

	// storage is the storage engine initially configured by the upper level TestSuite,
	// which either configures a memory or a rdbms storage backend.
	storage storage.Storage

	// txStorage storage is initialized from storage at the beginning of each test. If
	// the backend supports transactions, txStorage runs within a transaction.
	txStorage storage.Storage
}

func (suite *JobSuite) SetupTest() {
	suite.txStorage = common.InitStorage(suite.storage)
}

func (suite *JobSuite) TearDownTest() {
	common.FinalizeStorage(suite.txStorage)
}

func (suite *JobSuite) TestGetJobRequest() {
	t := suite.T()

	jobRequestFirst := job.Request{
		JobName:       "AName",
		Requestor:     "AIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorFirst,
	}
	jobIDa, err := suite.txStorage.StoreJobRequest(&jobRequestFirst)
	require.NoError(t, err)

	jobRequestSecond := job.Request{
		JobName:       "BName",
		Requestor:     "BIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorSecond,
	}
	jobIDb, err := suite.txStorage.StoreJobRequest(&jobRequestSecond)
	require.NoError(t, err)

	request, err := suite.txStorage.GetJobRequest(jobIDa)

	require.NoError(suite.T(), err)
	require.Equal(suite.T(), types.JobID(jobIDa), request.JobID)
	require.Equal(suite.T(), request.Requestor, "AIntegrationTest")
	require.Equal(suite.T(), request.JobDescriptor, jobDescriptorFirst)

	// Creation timestamp corresponds to the timestamp of the insertion into the
	// database. Assert that the timestamp retrieved from the database is within
	// and acceptable range
	require.True(t, request.RequestTime.After(time.Now().Add(-2*time.Second)))
	require.True(t, request.RequestTime.Before(time.Now().Add(2*time.Second)))

	request, err = suite.txStorage.GetJobRequest(jobIDb)

	// Creation timestamp corresponds to the timestamp of the insertion into the
	// database. Assert that the timestamp retrieved from the database is within
	// and acceptable range
	require.NoError(t, err)
	require.Equal(t, types.JobID(jobIDb), request.JobID)
	require.Equal(t, request.Requestor, "BIntegrationTest")
	require.Equal(t, request.JobDescriptor, jobDescriptorSecond)

	require.True(t, request.RequestTime.After(time.Now().Add(-2*time.Second)))
	require.True(t, request.RequestTime.Before(time.Now().Add(2*time.Second)))

}

func mustBuildQuery(t require.TestingT, queryFields ...storage.JobQueryField) *storage.JobQuery {
	jobQuery, err := storage.BuildJobQuery(queryFields...)
	require.NoError(t, err)
	return jobQuery
}

func (suite *JobSuite) TestListJobs() {
	t := suite.T()

	jobRequestFirst := job.Request{
		JobName:       "AName",
		Requestor:     "AIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorFirst,
	}
	jobIDa, err := suite.txStorage.StoreJobRequest(&jobRequestFirst)
	require.NoError(t, err)

	jobRequestSecond := job.Request{
		JobName:       "BName",
		Requestor:     "BIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorSecond,
	}
	jobIDb, err := suite.txStorage.StoreJobRequest(&jobRequestSecond)
	require.NoError(t, err)

	var res []types.JobID

	// No match criteria - returns all jobs.
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t))
	require.NoError(t, err)
	require.Equal(t, []types.JobID{jobIDa, jobIDb}, res)

	// List by states - matching any state is enough.
	// Both jobs are currently in Unknown state.
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobStates(job.JobStateCompleted, job.JobStateFailed),
	))
	require.NoError(t, err)
	require.Empty(t, res)
	require.NotNil(t, res)
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobStates(job.JobStateCompleted, job.JobStateFailed, job.JobStateUnknown),
	))
	require.NoError(t, err)
	require.Equal(t, []types.JobID{jobIDa, jobIDb}, res)

	// Tag matching - single tag.
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobTags("tests")))
	require.NoError(t, err)
	require.Equal(t, []types.JobID{jobIDa}, res)

	// Tag matching - must match all tags.
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobTags("integ", "tests")))
	require.NoError(t, err)
	require.Equal(t, []types.JobID{jobIDa}, res)
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobTags("integ", "tests", "no_such_tag")))
	require.NoError(t, err)
	require.Empty(t, res)
	require.NotNil(t, res)

	// Match by state - without any events both jobs are in unknown state.
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobStates(job.JobStateUnknown)))
	require.NoError(t, err)
	require.Equal(t, []types.JobID{jobIDa, jobIDb}, res)

	// Inject state events
	require.NoError(t, suite.txStorage.StoreFrameworkEvent(frameworkevent.Event{
		JobID: jobIDa, EventName: job.EventJobStarted, EmitTime: time.Unix(1, 0),
	}))
	require.NoError(t, suite.txStorage.StoreFrameworkEvent(frameworkevent.Event{
		JobID: jobIDb, EventName: job.EventJobStarted, EmitTime: time.Unix(2, 0),
	}))
	require.NoError(t, suite.txStorage.StoreFrameworkEvent(frameworkevent.Event{
		JobID: jobIDb, EventName: job.EventJobFailed, EmitTime: time.Unix(3, 0),
	}))
	require.NoError(t, suite.txStorage.StoreFrameworkEvent(frameworkevent.Event{
		JobID: jobIDa, EventName: job.EventJobCompleted, EmitTime: time.Unix(4, 0),
	}))

	// Multiple states - match any of them
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobStates(job.JobStateCompleted, job.JobStateFailed, job.JobStatePaused)))
	require.NoError(t, err)
	require.Equal(t, []types.JobID{jobIDa, jobIDb}, res)

	// State and tag match - must match both
	res, err = suite.txStorage.ListJobs(mustBuildQuery(t,
		storage.QueryJobStates(job.JobStateCompleted, job.JobStateFailed, job.JobStatePaused),
		storage.QueryJobTags("tests", "foo"),
	))
	require.NoError(t, err)
	require.Equal(t, []types.JobID{jobIDa}, res)
}
