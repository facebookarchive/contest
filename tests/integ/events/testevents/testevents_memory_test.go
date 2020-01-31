// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration

package test

import (
	"testing"

	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/stretchr/testify/suite"
)

func TestTestEventsSuiteMemoryStorage(t *testing.T) {

	testSuite := TestEventsSuite{}
	// Run the TestSuite with memory storage layer
	storagelayer := memory.New()
	testSuite.storage = storagelayer
	storage.SetStorage(storagelayer)

	suite.Run(t, &testSuite)
}
