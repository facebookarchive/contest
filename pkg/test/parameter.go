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
type Param struct {
	raw string
}

// NewParam initializes a new Param object from a string.
func NewParam(expression string) *Param {
	return &Param{raw: expression}
}

// UnmarshalJSON fills up a Param structure from the provided JSON byte stream.
func (p *Param) UnmarshalJSON(b []byte) error {
	// the string should be passed with the enclosing double-quotes, so strip
	// them first
	if len(b) < 2 {
		return fmt.Errorf("expected quoted string of length >=2, got %d", len(b))
	}
	if b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("expected quoted string, surrounding characters are not double-quotes")
	}
	b = b[1 : len(b)-1]
	p.raw = string(b)
	return nil
}

// MarshalJSON returns a JSON byte stream from a Param structure.
func (p Param) MarshalJSON() ([]byte, error) {
	return []byte(p.raw), nil
}

func (p Param) String() string {
	return p.Raw()
}

// Raw returns the raw, original string
func (p Param) Raw() string {
	return p.raw
}

// IsEmpty returns true if the original raw string is empty, false otherwise.
func (p Param) IsEmpty() bool {
	return p.raw == ""
}

// Expand evaluates the raw expression and applies the necessary manipulation,
// if any.
func (p *Param) Expand(target *target.Target) (string, error) {
	if p == nil {
		return "", errors.New("parameter cannot be nil")
	}
	// use Go text/template from here
	tmpl, err := template.New("").Funcs(getFuncMap()).Parse(p.raw)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, target); err != nil {
		return "", err
	}
	return buf.String(), nil
}
