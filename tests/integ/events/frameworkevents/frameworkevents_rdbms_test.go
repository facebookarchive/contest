// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration_storage

package test

import (
	"fmt"
	"testing"
	"time"

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
	storageLayer, err := common.NewStorage(opts...)
	if err != nil {
		panic(fmt.Sprintf("could not initialize rdbms storage layer: %v", err))
	}

	testSuite.storage = storageLayer
	suite.Run(t, &testSuite)
}
