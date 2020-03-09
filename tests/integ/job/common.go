// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"time"

	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/tests/integ/common"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var jobDescriptorFirst = `
  {
    "JobName": "FirstJob",
    "Runs": 1,
    "RunInterval": "5s",
    "Tags": ["integ", "tests"],
  }
`
var jobDescriptorSecond = `
  {
    "JobName": "SecondJob",
    "Runs": 1,
    "RunInterval": "5s",
  }
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

	jobRequestFirst := job.Request{
		JobName:       "AName",
		Requestor:     "AIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorFirst,
	}
	jobIDa, err := suite.txStorage.StoreJobRequest(&jobRequestFirst)
	require.NoError(suite.T(), err)

	jobRequestSecond := job.Request{
		JobName:       "BName",
		Requestor:     "BIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorSecond,
	}
	jobIDb, err := suite.txStorage.StoreJobRequest(&jobRequestSecond)
	require.NoError(suite.T(), err)

	request, err := suite.txStorage.GetJobRequest(types.JobID(jobIDa))

	require.NoError(suite.T(), err)
	require.Equal(suite.T(), types.JobID(jobIDa), request.JobID)
	require.Equal(suite.T(), request.Requestor, "AIntegrationTest")
	require.Equal(suite.T(), request.JobDescriptor, jobDescriptorFirst)

	// Creation timestamp corresponds to the timestamp of the insertion into the
	// database. Assert that the timestamp retrieved from the database is within
	// and acceptable range
	require.True(suite.T(), request.RequestTime.After(time.Now().Add(-2*time.Second)))
	require.True(suite.T(), request.RequestTime.Before(time.Now().Add(2*time.Second)))

	request, err = suite.txStorage.GetJobRequest(types.JobID(jobIDb))

	// Creation timestamp corresponds to the timestamp of the insertion into the
	// database. Assert that the timestamp retrieved from the database is within
	// and acceptable range
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), types.JobID(jobIDb), request.JobID)
	require.Equal(suite.T(), request.Requestor, "BIntegrationTest")
	require.Equal(suite.T(), request.JobDescriptor, jobDescriptorSecond)

	require.True(suite.T(), request.RequestTime.After(time.Now().Add(-2*time.Second)))
	require.True(suite.T(), request.RequestTime.Before(time.Now().Add(2*time.Second)))

}
