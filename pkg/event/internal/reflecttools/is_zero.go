// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package reflecttools

import (
	"reflect"
)

// IsZero checks if a value behind an interface equals to zero value of its type
func IsZero(v interface{}) bool {
	return reflect.ValueOf(v).IsZero()
}
