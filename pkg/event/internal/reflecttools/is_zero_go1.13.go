// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build go1.13

package reflecttools

import (
	"reflect"
)

func IsZero(v interface{}) bool {
	return reflect.ValueOf(v).IsZero()
}
