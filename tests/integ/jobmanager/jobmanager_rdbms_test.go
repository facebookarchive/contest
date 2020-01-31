// +build integration_storage

// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.
package test

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
	"github.com/facebookincubator/contest/tests/integ/common"

	"github.com/stretchr/testify/suite"
)

func TestJobManagerSuiteRdbmsStorage(t *testing.T) {
	testSuite := TestJobManagerSuite{}

	opts := []rdbms.Opt{
		rdbms.TestEventsFlushSize(1),
		rdbms.TestEventsFlushInterval(10 * time.Second),
	}
	storageLayer := common.NewStorage(opts...)
	storage.SetStorage(storageLayer)
	testSuite.storage = storageLayer

	suite.Run(t, &testSuite)
}
