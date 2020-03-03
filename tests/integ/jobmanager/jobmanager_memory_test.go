// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration

package test

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/targetlocker/inmemory"

	"github.com/stretchr/testify/suite"
)

func TestJobManagerSuiteMemoryStorage(t *testing.T) {

	testSuite := TestJobManagerSuite{}
	// Run the TestSuite with memory storage layer
	storagelayer := memory.New()
	testSuite.storage = storagelayer
	storage.SetStorage(storagelayer)

	targetLocker, err := (&inmemory.Factory{}).New(10 * time.Second, "")
	if err != nil {
		t.Fatal(err)
	}
	target.SetLocker(targetLocker)

	suite.Run(t, &testSuite)
}
