// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/facebookincubator/contest/pkg/event"
)

// EventTargetIn indicates that a target has entered a TestStep
var EventTargetIn = event.Name("TargetIn")

// EventTargetInErr indicates that a target has encountered an error while entering a TestStep
var EventTargetInErr = event.Name("TargetInErr")

// EventTargetOut indicates that a target has left a TestStep
var EventTargetOut = event.Name("TargetOut")

// EventTargetErr indicates that a target has encountered an error in a TestStep
var EventTargetErr = event.Name("TargetErr")

// EventTargetAcquired indicates that a target has been acquired for a Test
var EventTargetAcquired = event.Name("TargetAcquired")

// EventTargetAcquired indicates that a target has been released
var EventTargetReleased = event.Name("TargetReleased")

// ErrPayload represents the payload associated with a TargetErr event
type ErrPayload struct {
	Error string
}

// Target represents a target to run tests on.
// ID is required and must be unique. This is used to identify targets, some storage plugins use it as primary key.
// Common choices include IDs from inventory management systems, (fully qualified) hostnames, or textual representations of IP addresses.
// No assumptions about the format of IDs should be made (except being unique and not empty).
// FQDN, PrimaryIPv4, and PrimaryIPv6 are used by plugins to contact the target, set as many as possible for maximum plugin compatibility.
// Plugins are generally expected to attempt contacting devices via FQDN, IPv4, and IPv6. Note there is no way to enforce this and more specialized plugins might only support a subset.
type Target struct {
	ID          string `json:"ID"`
	FQDN        string `json:"FQDN,omitempty"`
	PrimaryIPv4 net.IP `json:"PrimaryIPv4,omitempty"`
	PrimaryIPv6 net.IP `json:"PrimaryIPv6,omitempty"`
	// This field is reserved for TargetManager to associate any state needed to keep track of the target between Acquire and Release.
	// It will be serialized between server restarts. Please keep it small.
	TargetManagerState json.RawMessage `json:"TMS,omitempty"`
}

// String produces a string representation for a Target.
func (t *Target) String() string {
	if t == nil {
		return "(*Target)(nil)"
	}
	var res strings.Builder
	res.WriteString(fmt.Sprintf(`Target{ID: "%s"`, t.ID))
	// deref params if they are set, to be more useful than %v
	if t.FQDN != "" {
		res.WriteString(fmt.Sprintf(`, FQDN: "%s"`, t.FQDN))
	}
	if t.PrimaryIPv4 != nil {
		res.WriteString(fmt.Sprintf(`, PrimaryIPv4: "%v"`, t.PrimaryIPv4))
	}
	if t.PrimaryIPv6 != nil {
		res.WriteString(fmt.Sprintf(`, PrimaryIPv6: "%v"`, t.PrimaryIPv6))
	}
	if len(t.TargetManagerState) > 0 {
		res.WriteString(fmt.Sprintf(`, TMS: "%s"`, t.TargetManagerState))
	}
	res.WriteString("}")
	return res.String()
}

// FilterTargets - Filter targets from targets based on targetIDs
func FilterTargets(targetIDs []string, targets []*Target) ([]*Target, error) {

	var targetList []*Target

	for _, targetID := range targetIDs {
		found := false
		for _, target := range targets {
			if target.ID == targetID {
				targetList = append(targetList, target)
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("unable to match target id %s to target", targetID)
		}
	}

	return targetList, nil
}
