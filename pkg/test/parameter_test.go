// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/stretchr/testify/require"
)

func TestParameterExpand(t *testing.T) {
	validExprs := [][4]string{
		// expression, target name, target ID, expected result
		[4]string{"{{ ToLower .Name }}", "www.slackware.IT", "1234", "www.slackware.it"},
		[4]string{"{{ .Name }}", "www.slackware.IT", "2345", "www.slackware.IT"},
		[4]string{"{{ .Name }}", "www.slackware.it", "3456", "www.slackware.it"},
		[4]string{"name={{ .Name }}, id={{ .ID }}", "blah", "12345", "name=blah, id=12345"},
	}
	for _, x := range validExprs {
		var p Param
		p.RawMessage = json.RawMessage(x[0])
		res, err := p.Expand(&target.Target{Name: x[1], ID: x[2]})
		require.NoError(t, err, x[0])
		require.Equal(t, x[3], res, x[0])
	}
}

func TestParameterExpandUserFunctions(t *testing.T) {
	title := func(a ...string) (string, error) {
		if len(a) == 0 {
			return "", errors.New("no params")
		}
		return strings.Title(a[0]), nil
	}
	err := RegisterFunction("title", title)
	require.NoError(t, err)
	validExprs := [][4]string{
		// expression, target name, target ID, expected result
		[4]string{"{{ Title .Name }}", "slackware.it", "1234", "Slackware.It"},
		[4]string{"{{ Title .ID }}", "slackware.it", "a1234a", "A1234a"},
	}
	for _, x := range validExprs {
		var p Param
		p.RawMessage = json.RawMessage(x[0])
		res, err := p.Expand(&target.Target{Name: x[1], ID: x[2]})
		require.NoError(t, err, x[0])
		require.Equal(t, x[3], res, x[0])
	}
}
