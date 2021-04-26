// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNilTarget(t *testing.T) {
	var recoverResult interface{}
	func() {
		defer func() {
			recoverResult = recover()
		}()
		_ = (*Target)(nil).String()
	}()
	require.Nil(t, recoverResult)
}

func TestTargetStringification(t *testing.T) {
	var t0 *Target
	require.Equal(t, `(*Target)(nil)`, t0.String())

	t1 := &Target{ID: "123"}
	require.Equal(t, `Target{ID: "123"}`, t1.String())
	tj1, _ := json.Marshal(t1)
	require.Equal(t, `{"ID":"123"}`, string(tj1))

	t2 := &Target{FQDN: "example.com"}
	require.Equal(t, `Target{ID: "", FQDN: "example.com"}`, t2.String())
	tj2, _ := json.Marshal(t2)
	require.Equal(t, `{"ID":"","FQDN":"example.com"}`, string(tj2))

	t3 := &Target{ID: "123", FQDN: "example.com", PrimaryIPv4: net.IPv4(1, 2, 3, 4)}
	require.Equal(t, `Target{ID: "123", FQDN: "example.com", PrimaryIPv4: "1.2.3.4"}`, t3.String())
	tj3, _ := json.Marshal(t3)
	require.Equal(t, `{"ID":"123","FQDN":"example.com","PrimaryIPv4":"1.2.3.4"}`, string(tj3))

	t4 := &Target{ID: "123", FQDN: "example.com", PrimaryIPv6: net.IPv6loopback}
	require.Equal(t, `Target{ID: "123", FQDN: "example.com", PrimaryIPv6: "::1"}`, t4.String())
	tj4, _ := json.Marshal(t4)
	require.Equal(t, `{"ID":"123","FQDN":"example.com","PrimaryIPv6":"::1"}`, string(tj4))

	t5 := &Target{ID: "123", TargetManagerState: json.RawMessage([]byte(`{"hello": "world"}`))}
	require.Equal(t, `Target{ID: "123", TMS: "{"hello": "world"}"}`, t5.String())
	tj5, _ := json.Marshal(t5)
	require.Equal(t, `{"ID":"123","TMS":{"hello":"world"}}`, string(tj5))
}
