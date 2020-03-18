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
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"
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

	targetLocker, err := inmemory.New(10 * time.Second, "")
	if err != nil {
		t.Fatal(err)
	}
	target.SetLocker(targetLocker)

	suite.Run(t, &testSuite)
}
