// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type paramSubStructure struct {
	Val1 		 string
	Val2 		 string
	More_nesting map[string]string
}

func TestTestStepParametersUnmarshalNested(t *testing.T) {
	descriptor := `{
		"str": ["some string"],
		"num": [12],
		"substruct": [{
			"Val1": "foo",
			"Val2": "bar",
			"More_nesting": {
				"foobar": "baz"
			}
		}]
	}`

	var params TestStepParameters
	err := json.Unmarshal([]byte(descriptor), &params)
	require.NoError(t, err)
	require.Equal(t, "some string", params.GetOne("str").String())
	num, err := params.GetInt("num")
	require.NoError(t, err)
	require.Equal(t, int64(12), num)

	// must be able to unmarshal substruct futher
	var substruct paramSubStructure
	err = json.Unmarshal(params.GetOne("substruct").JSON(), &substruct)
	require.NoError(t, err)
	require.Equal(t, "foo", substruct.Val1)
	require.Equal(t, "bar", substruct.Val2)
	require.Equal(t, "baz", substruct.More_nesting["foobar"])
}
