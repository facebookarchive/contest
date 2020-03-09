// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration

package test

import (
	"fmt"
	"testing"

	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/stretchr/testify/suite"
)

func TestJobSuiteMemoryStorage(t *testing.T) {

	testSuite := JobSuite{}
	// Run the TestSuite with memory storage layer
	storagelayer, err := memory.New()
	if err != nil {
		panic(fmt.Sprintf("could not initialize in-memory storage layer: %v", err))
	}
	testSuite.storage = storagelayer
	suite.Run(t, &testSuite)
}
