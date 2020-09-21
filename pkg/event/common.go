// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package event

import (
	"fmt"
	"regexp"

	"github.com/facebookincubator/contest/pkg/storage/limits"
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
		return fmt.Errorf("event name %s does not comply with events api (does not match %s)", AllowedEventFormat.String(), string(e))
	}
	if err := limits.Validator.ValidateEventName(string(e)); err != nil {
		return err
	}
	return nil
}
