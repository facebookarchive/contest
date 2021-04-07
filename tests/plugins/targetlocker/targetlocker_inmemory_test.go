// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetlocker

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"
)

type InMemoryTargetLockerTestSuite struct {
	TargetLockerTestSuite
}

func (ts *InMemoryTargetLockerTestSuite) SetupTest() {
	ts.clock = clock.NewMock()
	require.NotNil(ts.T(), ts.clock)
	ts.clock.Add(1 * time.Hour) // avoid zero time, start at 1:00
	ts.tl = inmemory.New(ts.clock)
	require.NotNil(ts.T(), ts.tl)
}

func (ts *InMemoryTargetLockerTestSuite) TearDownTest() {
	ts.tl.Close()
	ts.tl = nil
}

func TestInMemoryTargetLocker(t *testing.T) {
	suite.Run(t, &InMemoryTargetLockerTestSuite{})
}
