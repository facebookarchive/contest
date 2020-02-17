// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/test"
)

type ErrStepLabelIsMandatory struct {
	TestStepDescriptor test.TestStepDescriptor
}

func (err ErrStepLabelIsMandatory) Error() string {
	return fmt.Sprintf("step has no label, but it is mandatory (step: %+v)", err.TestStepDescriptor)
}
