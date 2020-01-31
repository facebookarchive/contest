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
	storage storage.Storage
}

func (suite *JobSuite) TearDownTest() {
	suite.storage.Reset()
}

func populateJob(backend storage.Storage) error {

	jobRequestFirst := job.Request{
		JobName:       "AName",
		Requestor:     "AIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorFirst,
	}
	_, err := backend.StoreJobRequest(&jobRequestFirst)
	if err != nil {
		return err
	}

	jobRequestSecond := job.Request{
		JobName:       "BName",
		Requestor:     "BIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptorSecond,
	}
	_, err = backend.StoreJobRequest(&jobRequestSecond)
	return err
}

func (suite *JobSuite) TestPersistJobRequestReturnsCorrectID() {
	jobDescriptor := "{'JobName': 'Test'}"
	jobRequestFirst := job.Request{
		JobName:       "AName",
		Requestor:     "AIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptor,
	}

	jobID, err := suite.storage.StoreJobRequest(&jobRequestFirst)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), types.JobID(1), jobID)

	jobRequestSecond := job.Request{
		JobName:       "BName",
		Requestor:     "BIntegrationTest",
		RequestTime:   time.Now(),
		JobDescriptor: jobDescriptor,
	}

	jobID, err = suite.storage.StoreJobRequest(&jobRequestSecond)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), types.JobID(2), jobID)
}

func (suite *JobSuite) TestGetJobRequest() {

	populateJob(suite.storage)

	request, err := suite.storage.GetJobRequest(types.JobID(1))
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), types.JobID(1), request.JobID)
	require.Equal(suite.T(), request.Requestor, "AIntegrationTest")
	require.Equal(suite.T(), request.JobDescriptor, jobDescriptorFirst)

	// Creation timestamp corresponds to the timestamp of the insertion into the
	// database. Assert that the timestamp retrieved from the database is within
	// and acceptable range
	require.True(suite.T(), request.RequestTime.After(time.Now().Add(-2*time.Second)))
	require.True(suite.T(), request.RequestTime.Before(time.Now().Add(2*time.Second)))

	request, err = suite.storage.GetJobRequest(types.JobID(2))

	// Creation timestamp corresponds to the timestamp of the insertion into the
	// database. Assert that the timestamp retrieved from the database is within
	// and acceptable range
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), types.JobID(2), request.JobID)
	require.Equal(suite.T(), request.Requestor, "BIntegrationTest")
	require.Equal(suite.T(), request.JobDescriptor, jobDescriptorSecond)

	require.True(suite.T(), request.RequestTime.After(time.Now().Add(-2*time.Second)))
	require.True(suite.T(), request.RequestTime.Before(time.Now().Add(2*time.Second)))

}
