// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package abstract

import (
	"strings"

	"github.com/facebookincubator/contest/pkg/event"
)

// Factory is the common part of interface for any factory
type Factory interface {
	// UniqueImplementationName returns the Name of the implementation
	// unique among all reachable implementation of specific type of factories
	// (names are case insensitive).
	UniqueImplementationName() string
}

// Factories is a helper-slice of Factory-ies which provides common routines.
type Factories []Factory

// ToMap returns a map of factories with an implementation name as the key
func (s Factories) ToMap() map[string]Factory {
	result := make(map[string]Factory, len(s))
	for _, factory := range s {
		result[strings.ToLower(factory.UniqueImplementationName())] = factory
	}
	return result
}

// UniqueImplementationNames calls UniqueImplementationName on each LockerFactory
// and returns the slice of results.
func (s Factories) UniqueImplementationNames() (result []string) {
	result = make([]string, 0, len(s))
	for _, factory := range s {
		result = append(result, factory.UniqueImplementationName())
	}
	return
}

// UniqueImplementationNamesLower does the same as UniqueImplementationNames
// but all values are lowered (with strings.ToLower).
func (s Factories) UniqueImplementationNamesLower() (result []string) {
	result = make([]string, 0, len(s))
	for _, factory := range s {
		result = append(result, strings.ToLower(factory.UniqueImplementationName()))
	}
	return
}

func (s Factories) String() string {
	return strings.Join(s.UniqueImplementationNames(), `, `)
}

// Find returns the factory with the UniqueImplementationName equals to
// the argument (case insensitive).
//
// T: O(n)
func (s Factories) Find(uniqueImplName string) Factory {
	uniqueImplName = strings.ToLower(uniqueImplName)
	for _, factory := range s {
		if strings.ToLower(factory.UniqueImplementationName()) == uniqueImplName {
			return factory
		}
	}
	return nil
}

// FactoryWithEvents is an abstract factory which also may emit events
type FactoryWithEvents interface {
	Factory

	// Events is used by the framework to determine which events this plugin will
	// emit. Any emitted event that is not registered here will cause the plugin to
	// fail.
	Events() []event.Name
}
