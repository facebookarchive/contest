// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build go1.13

package event

import (
	"reflect"
)

func (queryFields QueryFields) validateNoZeroValues() error {
	for _, queryField := range queryFields {
		if reflect.ValueOf(queryField).IsZero() {
			return ErrQueryFieldHasZeroValue{QueryField: queryField}
		}
	}
	return nil
}
