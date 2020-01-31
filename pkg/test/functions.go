// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"fmt"
	"strings"
	"sync"
)

// funcMap is a map between function name and its implementation.
var funcMap = map[string]interface{}{
	// some common pre-sets
	"ToUpper": strings.ToUpper,
	"ToLower": strings.ToLower,
	"Title":   strings.Title,
}
var funcMapMutex sync.Mutex

// getFuncMap returns a copy of funcMap that can be passed to Template.Funcs.
// The map is copied so it can be passed safely even if the original is modified.
func getFuncMap() map[string]interface{} {
	mapCopy := make(map[string]interface{}, len(funcMap))
	funcMapMutex.Lock()
	defer funcMapMutex.Unlock()
	for k, v := range funcMap {
		mapCopy[k] = v
	}
	return mapCopy
}

// RegisterFunction registers a template function suitable for text/template.
// It can be either a func(string) string or a func(string) (string, error),
// hence it's passed as an empty interface.
func RegisterFunction(name string, fn interface{}) error {
	funcMapMutex.Lock()
	defer funcMapMutex.Unlock()
	if funcMap == nil {
		funcMap = make(map[string]interface{})
	}
	if _, ok := funcMap[name]; ok {
		return fmt.Errorf("function '%s' is already registered", name)
	}
	funcMap[name] = fn
	return nil
}
