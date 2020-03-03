// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"strings"
	"testing"

	"github.com/facebookincubator/contest/pkg/abstract"
)

func TestValidateFactories(t *testing.T) {
	for _, factories := range []abstract.Factories{
		targetLockerFactories.ToAbstract(),
	} {
		// Check name uniqueness
		m := map[string]struct{}{}
		for _, uniqueNameRaw := range factories.UniqueImplementationNames() {
			uniqueName := strings.ToLower(uniqueNameRaw)
			if _, alreadyExists := m[uniqueName]; alreadyExists {
				t.Errorf("factory %s is defined multiple times", uniqueNameRaw)
			}
			m[uniqueName] = struct{}{}
		}
	}
}
