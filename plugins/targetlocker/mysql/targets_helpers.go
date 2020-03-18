// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package mysql implements target.Locker using a MySQL table as the backend storage.
// The same table could be safely used by multiple instances of ConTest.
package mysql

import (
	"sort"

	"github.com/facebookincubator/contest/pkg/target"
)

// targetsIDs return the slice of ID values.
// It includes the ID of every Target (of `[]*target.Target`) is the same order, even
// if there is a duplication.
//
// Does not support "nil" Target-s.
func targetsIDs(s []*target.Target) (result []string) {
	result = make([]string, 0, len(s))
	for _, target := range s {
		result = append(result, target.ID)
	}
	return
}

// targetsSort performs sort of the targets by ID
func targetsSort(s []*target.Target) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].ID < s[j].ID
	})
}

// targetsSearch performs sort.Search on Targets in attempt to
// find the index of the target with ID closest to the `targetID`.
//
// Warning! Targets should be sorted first! See method targetsSort().
func targetsSearch(s []*target.Target, targetID string) int {
	return sort.Search(len(s), func(i int) bool {
		return s[i].ID >= targetID
	})
}

// Find returns a target by targetID, or nil if not found.
//
// Warning! Targets should be sorted first! See method targetsSort().
func targetsFind(s []*target.Target, targetID string) *target.Target {
	idx := targetsSearch(s, targetID)
	if idx < 0 || idx >= len(s) {
		return nil
	}
	target := s[idx]
	if target.ID != targetID {
		return nil
	}
	return target
}
