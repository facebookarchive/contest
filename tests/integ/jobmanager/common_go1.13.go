// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage
// +build go1.13

package test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireErrorType(t *testing.T, err, errType error) {
	require.True(t, errors.As(err, &errType))
}