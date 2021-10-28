// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package exec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockEmitter struct {
	mock.Mock

	Calls []testevent.Data
}

func (e *mockEmitter) Emit(ctx xcontext.Context, data testevent.Data) error {
	e.Calls = append(e.Calls, data)

	args := e.Called(ctx, data)
	return args.Error(0)
}

func TestOCPEventParser(t *testing.T) {
	ctx := xcontext.Background()

	ev := &mockEmitter{}
	ev.On("Emit", ctx, mock.Anything).Return(nil)

	data := `{"testRunArtifact":{"testRunEnd":{"name":"Error Monitor","status":"COMPLETE","result":"PASS"}},"sequenceNumber":1,"timestamp":"ts"}`
	r := strings.NewReader(data)

	p := NewOCPEventParser(nil, ev)
	dec := json.NewDecoder(r)
	for dec.More() {
		var root *OCPRoot
		err := dec.Decode(&root)
		require.NoError(t, err)

		err = p.Parse(ctx, root)
		require.NoError(t, err)
	}

	require.Equal(t, 1, len(ev.Calls))
	require.Equal(t, ev.Calls[0].EventName, TestEndEvent)

	var payload testEndEventPayload
	err := json.Unmarshal(*ev.Calls[0].Payload, &payload)
	require.NoError(t, err)

	require.Equal(t, payload.Result, "PASS")
}
