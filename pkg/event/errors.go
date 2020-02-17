// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package event

import (
	"fmt"
)

// ErrQueryFieldPassedTwice is returned when QueryFields failed validation due
// to multiple QueryField which modifies the same field (this is unexpected
// and forbidden).
type ErrQueryFieldPassedTwice struct {
	QueryField QueryField
}

func (err ErrQueryFieldPassedTwice) Error() string {
	return fmt.Sprintf("field %T is passed twice", err.QueryField)
}

// ErrQueryFieldHasZeroValue is returned when a QueryFields failed validation
// due to a QueryField with a zero value (this is unexpected and forbidden).
type ErrQueryFieldHasZeroValue struct {
	QueryField QueryField
}

func (err ErrQueryFieldHasZeroValue) Error() string {
	return fmt.Sprintf("field %T has a zero value", err.QueryField)
}
