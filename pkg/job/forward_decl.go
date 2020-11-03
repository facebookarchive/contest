// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"github.com/facebookincubator/contest/pkg/test"
)

// --- "Forward declaration" from #118 to enable building DB migration logic ----
type ExtendedDescriptor struct {
	JobDescriptor
	StepsDescriptors []test.StepsDescriptors
}

// --- End Forward declaration from #118 ----
