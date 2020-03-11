// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package event

import (
	"fmt"
)

// ErrInvalidEventName means event.Name has invalid value,
// see event.Name.Validate().
type ErrInvalidEventName struct {
	EventName Name
}

func (err ErrInvalidEventName) Error() string {
	return fmt.Sprintf("event name '%s' does not comply with events API (does not match regular expression '%s')",
		err.EventName, AllowedEventFormat.String())
}
