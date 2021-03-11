// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logger

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConvertLogger(t *testing.T) {
	require.NotNil(t, ConvertLogger(fmt.Printf))
	require.NotNil(t, ConvertLogger(logrus.New()))
	require.NotNil(t, ConvertLogger(zap.NewExample().Sugar()))
	require.Nil(t, ConvertLogger(zap.NewExample()))
}
