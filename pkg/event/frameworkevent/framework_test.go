// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build go1.13

package frameworkevent_test

import (
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/stretchr/testify/assert"

	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
)

func TestBuildQuery_Positive(t *testing.T) {
	_, err := frameworkevent.QueryFields{
		frameworkevent.QueryJobID(1),
		frameworkevent.QueryEventNames([]event.Name{"unit-test"}),
		frameworkevent.QueryEmittedStartTime(time.Now()),
		frameworkevent.QueryEmittedEndTime(time.Now()),
	}.BuildQuery()
	assert.NoError(t, err)
}
