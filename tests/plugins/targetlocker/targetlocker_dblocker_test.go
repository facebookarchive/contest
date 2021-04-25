// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration_storage

package targetlocker

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/facebookincubator/contest/plugins/targetlocker/dblocker"
	"github.com/facebookincubator/contest/tests/integ/common"
)

type DBLockerTestSuite struct {
	TargetLockerTestSuite
}

func (ts *DBLockerTestSuite) SetupTest() {
	ts.clock = clock.NewMock()
	require.NotNil(ts.T(), ts.clock)
	ts.clock.Add(1 * time.Hour) // avoid zero time, start at 1:00
	tl, err := dblocker.New(
		common.GetDatabaseURI(),
		dblocker.WithClock(ts.clock),
		dblocker.WithMaxBatchSize(3),
	)
	require.NoError(ts.T(), err)
	require.NotNil(ts.T(), tl)
	tl.ResetAllLocks(ctx)
	ts.tl = tl
}

func (ts *DBLockerTestSuite) TearDownTest() {
	ts.tl.Close()
	ts.tl = nil
}

func TestDBLocker(t *testing.T) {
	suite.Run(t, &DBLockerTestSuite{})
}
