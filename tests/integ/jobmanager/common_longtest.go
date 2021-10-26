// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage
// +build longtest

package test

import (
	"syscall"
	"time"

	"github.com/facebookincubator/contest/pkg/jobmanager"
)

func (suite *TestJobManagerSuite) TestWaitAndExit() {
	suite.testExit(syscall.SIGUSR1, jobmanager.EventJobCompleted, 10*time.Second)
}
