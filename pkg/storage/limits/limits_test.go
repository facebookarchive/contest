// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package limits_test

import (
	"strings"
	//"fmt"
	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"

	//"github.com/facebookincubator/contest/pkg/target"
	//"github.com/facebookincubator/contest/pkg/test"
	"github.com/stretchr/testify/assert"
	//"github.com/stretchr/testify/require"
)

func TestServerIDPanics(t *testing.T) {
	apiInst := api.New(func() string { return strings.Repeat("A", 65) })
	assert.Panics(t, func() { apiInst.ServerID() })
}

type eventMsg struct {
	requestor api.EventRequestor
}

func (e eventMsg) Requestor() api.EventRequestor { return e.requestor }
func TestRequestorName(t *testing.T) {
	apiInst := api.API{}
	timeout := time.Second
	err := apiInst.SendEvent(&api.Event{Msg: eventMsg{requestor: api.EventRequestor(strings.Repeat("A", 33))}}, &timeout)
	assert.Error(t, err)
}

func TestEventName(t *testing.T) {
	eventName := event.Name(strings.Repeat("A", 33))
	err := eventName.Validate()
	assert.Error(t, err)
}
