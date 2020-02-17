// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package event

import (
	"reflect"
)

// QueryField is an abstraction over frameworkevent.QueryField and testevent.QueryField
type QueryField interface {
}

// QueryField is an abstraction over frameworkevent.QueryFields and testevent.QueryFields
type QueryFields []QueryField

func (queryFields QueryFields) validateNoDuplicates() error {
	alreadyUsedField := map[reflect.Type]bool{}
	for _, queryField := range queryFields {
		queryFieldType := reflect.TypeOf(queryField)
		if alreadyUsedField[queryFieldType] {
			return ErrQueryFieldPassedTwice{QueryField: queryField}
		}
		alreadyUsedField[queryFieldType] = true
	}
	return nil
}

// Validate returns an error if a query cannot be formed using this queryFields.
func (queryFields QueryFields) Validate() error {
	if err := queryFields.validateNoDuplicates(); err != nil {
		return err
	}
	if err := queryFields.validateNoZeroValues(); err != nil {
		return err
	}
	return nil
}
