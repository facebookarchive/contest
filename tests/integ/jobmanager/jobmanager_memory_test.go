// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration

package test

import (
	"fmt"
	"testing"

	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/plugins/storage/memory"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestJobManagerSuiteMemoryStorage(t *testing.T) {

	testSuite := TestJobManagerSuite{}
	// Run the TestSuite with memory storage layer
	storagelayer, err := memory.New()
	if err != nil {
		panic(fmt.Sprintf("could not initialize in-memory storage layer: %v", err))

	}
	testSuite.storage = storagelayer
	err = storage.SetStorage(storagelayer)
	require.NoError(t, err)

	suite.Run(t, &testSuite)
}
