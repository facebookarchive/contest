// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"

	"github.com/facebookincubator/contest/pkg/target"
)

// NewParam inititalizes a new Param object from a string
func NewParam(s string) Param {
	var p Param
	p.RawMessage = json.RawMessage(s)
	return p
}

// Param represents a test step parameter. It is initialized from JSON,
// and can be either a strings or more complex JSON structures.
// Plugins are expected to know which one they expect and use the
// provided convenience functions to obtain either the string or
// json.RawMessage representation.
type Param struct {
	json.RawMessage
}

// IsEmpty returns true if the original raw string is empty, false otherwise.
func (p Param) IsEmpty() bool {
	return p.String() == ""
}

func (p Param) String() string {
	var str string
	if json.Unmarshal(p.RawMessage, &str) == nil {
		return str
	}
	// can't unmarshal to string, return raw json
	return string(p.RawMessage)
}

func (p *Param) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &p.RawMessage)
}

// Expand evaluates the raw expression and applies the necessary manipulation,
// if any.
func (p *Param) Expand(target *target.Target) (string, error) {
	if p == nil {
		return "", errors.New("parameter cannot be nil")
	}
	// use Go text/template from here
	tmpl, err := template.New("").Funcs(getFuncMap()).Parse(p.String())
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, target); err != nil {
		return "", err
	}
	return buf.String(), nil
}
