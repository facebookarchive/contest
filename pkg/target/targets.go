// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package target

import (
	"sort"
)

// Targets is a helper to manipulate with a slice of Targets
// (it collects common routines for multiple Targets).
type Targets []*Target

// IDs return the slice of ID values.
// It includes the ID of every Target (of `Targets`) is the same order, even
// if there is a duplication.
//
// Does not support "nil" Target-s.
func (s Targets) IDs() (result []string) {
	result = make([]string, 0, len(s))
	for _, target := range s {
		result = append(result, target.ID)
	}
	return
}

func (s Targets) Len() int           { return len(s) }
func (s Targets) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s Targets) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Sort performs sort of the targets by ID
func (s Targets) Sort() {
	sort.Sort(s)
}

// Search performs sort.Search on Targets in attempt to
// find the index of the target with ID closest to the `targetID`.
//
// Warning! Targets should be sorted first! See method Targets.Sort().
func (s Targets) Search(targetID string) int {
	return sort.Search(s.Len(), func(i int) bool {
		return s[i].ID >= targetID
	})
}

// Find returns a target by targetID.
//
// Warning! Targets should be sorted first! See method Targets.Sort().
func (s Targets) Find(targetID string) *Target {
	idx := s.Search(targetID)
	if idx < 0 || idx >= s.Len() {
		return nil
	}
	target := s[idx]
	if target.ID != targetID {
		return nil
	}
	return target
}
