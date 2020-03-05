// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package event

import (
	"regexp"
)

// AllowedEventFormat defines the allowed format for an event
var AllowedEventFormat = regexp.MustCompile(`^[a-zA-Z]+$`)

// Name is a custom type which represents the name of an event
type Name string

// StateEventName is the name of the event used to emit state information
var StateEventName = Name("TestState")

// Validate validates that the event name conforms with the framework API
func (e Name) Validate() error {
	matched := AllowedEventFormat.MatchString(string(e))
	if !matched {
		return ErrInvalidEventName{EventName: e}
	}
	return nil
}

// Names is a helper-slice for multiple `Name`-s.
type Names []Name

// Validate performs method Validate for each Name.
func (s Names) Validate() error {
	for _, name := range s {
		if err := name.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ToMap returns a map of existing names to empty structs.
func (s Names) ToMap() map[Name]struct{} {
	result := make(map[Name]struct{}, len(s))
	for _, name := range s {
		result[name] = struct{}{}
	}
	return result
}
