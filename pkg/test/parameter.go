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

// Param represents a test step parameter. It is initialized from JSON,
// and can be either a strings or more complex JSON structures.
// Plugins are expected to know which one they expect and use the
// provided convenience functions to obtain either the string or
// json.RawMessage representation.
// Simple strings can contain simple function expressions that can
// be expanded, for example to include the hostname of the test target.
type Param json.RawMessage

// IsEmpty returns true if the original raw string is empty, false otherwise.
func (p Param) IsEmpty() bool {
	return p.String() == ""
}

// String returns the parameter as a string. This helper never fails.
// If the underlying JSON cannot be unmarshalled into a simple string,
// this function returns a string representation of the JSON structure.
func (p Param) String() string {
	var s string
	if err := json.Unmarshal(p, &s); err == nil {
		return s
	}
	// full JSON
	return string(p)
}

// JSON returns the parameter as json.RawMessage for further
// unmarshalling by the plugin.
func (p Param) JSON() json.RawMessage {
	return json.RawMessage(p)
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
