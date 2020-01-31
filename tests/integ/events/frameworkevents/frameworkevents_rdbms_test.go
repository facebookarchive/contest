// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration_storage

package test

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
	"github.com/facebookincubator/contest/tests/integ/common"
	"github.com/stretchr/testify/suite"
)

func TestFrameworkEventsSuiteRdbmsStorage(t *testing.T) {

	testSuite := FrameworkEventsSuite{}

	opts := []rdbms.Opt{
		rdbms.FrameworkEventsFlushSize(0),
		rdbms.FrameworkEventsFlushInterval(10 * time.Second),
	}
	storageLayer := common.NewStorage(opts...)
	storage.SetStorage(storageLayer)
	testSuite.storage = storageLayer

	suite.Run(t, &testSuite)
}
