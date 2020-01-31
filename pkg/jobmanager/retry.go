// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"fmt"
	"github.com/facebookincubator/contest/pkg/api"
)

func (jm *JobManager) retry(ev *api.Event) *api.EventResponse {
	return &api.EventResponse{
		Requestor: ev.Msg.Requestor(),
		Err:       fmt.Errorf("Not implemented"),
	}
}
