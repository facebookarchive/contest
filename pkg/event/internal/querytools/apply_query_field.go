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

// ApplyQueryField sets value `queryField` to `fieldPtr`, in other words
// it is a smart equivalent of: `*fieldPtr = queryField`.
//
// Besides of assigning the value this function also:
//
// * Checks if the field is already set. In this case error
// ErrQueryFieldIsAlreadySet is returned.
// * Checks if `queryField` contains a zero-value of the field. In this case
// error ErrQueryFieldHasZeroValue is returned.
func ApplyQueryField(fieldPtr interface{}, queryField event.QueryField) error {
	// A zero-valued `queryField` has no sense in here (it won't
	// affect the Query). So if `queryField` is a zero-value then
	// return an error.
	if reflecttools.IsZero(queryField) {
		return event.ErrQueryFieldHasZeroValue{QueryField: queryField}
	}

	// `fieldPtr` is always a pointer to a field, so here we
	// declare `field` as `*fieldPtr`.
	field := reflect.ValueOf(fieldPtr).Elem()

	// If `*fieldPtr` already contains a non-zero value then return an error.
	if !reflecttools.IsZero(field.Interface()) {
		return event.ErrQueryFieldIsAlreadySet{FieldValue: field.Interface(), QueryField: queryField}
	}

	// The part: *fieldPtr = queryField
	field.Set(reflect.ValueOf(queryField).Convert(field.Type()))
	return nil
}
