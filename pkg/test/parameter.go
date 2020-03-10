// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/facebookincubator/contest/pkg/target"
)

// Param represents a test step parameter. It is initialized from a string,
// which can be either a host name, a function expression, or just any regular
// string. It enables hostname and function expression validation and a few
// other validation operations. This is meant to simplify the way plugin access
// and use test step parameters.
type Param string

// IsEmpty returns true if the original raw string is empty, false otherwise.
func (p Param) IsEmpty() bool {
	return p == ""
}

// Expand evaluates the raw expression and applies the necessary manipulation,
// if any.
func (p *Param) Expand(target *target.Target) (string, error) {
	if p == nil {
		return "", errors.New("parameter cannot be nil")
	}
	// use Go text/template from here
	tmpl, err := template.New("").Funcs(getFuncMap()).Parse(string(*p))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, target); err != nil {
		return "", err
	}
	return buf.String(), nil
}
