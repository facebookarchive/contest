// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build go1.13

package testevent_test

import (
	"errors"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	. "github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/stretchr/testify/assert"
)

func TestBuildQuery_Positive(t *testing.T) {
	_, err := QueryFields{
		QueryJobID(1),
		QueryEventNames([]event.Name{"unit-test"}),
		QueryTestStepLabel("unit-test"),
		QueryEmittedStartTime(time.Now()),
		QueryEmittedEndTime(time.Now()),
		QueryRunID(1),
	}.BuildQuery()
	assert.NoError(t, err)
}

func TestBuildQuery_NoDups(t *testing.T) {
	_, err := QueryFields{
		QueryJobID(2),
		QueryEmittedStartTime(time.Now()),
		QueryEmittedStartTime(time.Now()),
	}.BuildQuery()
	assert.Error(t, err)
	assert.True(t, errors.As(err, &event.ErrQueryFieldIsAlreadySet{}))
}

func TestBuildQuery_NoZeroValues(t *testing.T) {
	_, err := QueryFields{
		QueryJobID(0),
		QueryEmittedStartTime(time.Now()),
		QueryEmittedEndTime(time.Now()),
	}.BuildQuery()
	assert.Error(t, err)
	assert.True(t, errors.As(err, &event.ErrQueryFieldHasZeroValue{}))
}
