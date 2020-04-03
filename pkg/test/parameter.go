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

// NewParam inititalizes a new Param object directly from a string.
// Note that no validation is performed if the input is actually valid JSON.
func NewParam(s string) *Param {
	var p Param
	p.RawMessage = json.RawMessage(s)
	return &p
}

// Param represents a test step parameter. It is initialized from JSON,
// and can be a string or a more complex JSON structure.
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

// String returns the parameter as a string. This helper never fails.
// If the underlying JSON cannot be unmarshalled into a simple string,
// this function returns a string representation of the JSON structure.
func (p Param) String() string {
	var str string
	if json.Unmarshal(p.RawMessage, &str) == nil {
		return str
	}
	// can't unmarshal to string, return raw json
	return string(p.RawMessage)
}
// JSON returns the parameter as json.RawMessage for further
// unmarshalling by the test plugin.
func (p Param) JSON() json.RawMessage {
	return p.RawMessage
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
