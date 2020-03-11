// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"reflect"
	"testing"

	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateFactories(t *testing.T) {
	// See also pluginregistry.TestNewPluginRegistry_RegisterFactory
	err := setupPluginRegistry()
	if err != nil {
		log.Fatal(err)
	}

	// Check if every used factory is on it's place (nothing lost and no types were mixed up).
	typeMap := map[reflect.Type]pluginregistry.FactoryType{}
	for _, factory := range allFactories() {
		factoryType := pluginregistry.FactoryTypeUndefined
		for factoryTypeTry := pluginregistry.FactoryTypeUndefined + 1; factoryTypeTry <= pluginregistry.MaxFactoryType; factoryTypeTry++ {
			cmpFactory, err := pluginRegistry.Factory(factoryTypeTry, factory.UniqueImplementationName())
			switch err.(type) {
			case nil:
			case pluginregistry.ErrFactoryNotFoundByName:
				continue
			default:
				t.Error(err)
			}
			if factory == cmpFactory {
				factoryType = factoryTypeTry
				break
			}
		}
		if !assert.NotEqual(t, pluginregistry.FactoryTypeUndefined, factoryType) {
			continue
		}
		newMethod, ok := reflect.TypeOf(factory).MethodByName(`New`)
		require.True(t, ok)
		objType := newMethod.Type.Out(0)
		if typeMap[objType] == 0 {
			typeMap[objType] = factoryType
			continue
		}
		assert.Equal(t, typeMap[objType], factoryType)
	}
	require.LessOrEqual(t, len(typeMap), pluginregistry.MaxFactoryType+1)
}
