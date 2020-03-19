// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package querytools

import (
	"reflect"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/internal/reflecttools"
)

func ApplyQueryField(fieldPtr interface{}, queryField event.QueryField) error {
	if reflecttools.IsZero(queryField) {
		return event.ErrQueryFieldHasZeroValue{QueryField: queryField}
	}
	field := reflect.ValueOf(fieldPtr).Elem()
	if !reflecttools.IsZero(field.Interface()) {
		return event.ErrQueryFieldIsAlreadySet{FieldValue: field.Interface(), QueryField: queryField}
	}
	field.Set(reflect.ValueOf(queryField).Convert(field.Type()))
	return nil
}
