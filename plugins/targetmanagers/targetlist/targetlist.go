// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package targetlist implements a simple target manager that contains a static
// list of targets. Use it as follows in a job descriptor:
// "TargetManager": "targetlist",
// "TargetManagerAcquireParameters": {
//     "Targets": [
//         {
//             "Name": "hostname1.example.com",
//             "ID": "id1"
//         },
//         {
//             "Name": "hostname2.example.com",
//             "ID": "id2"
//         }
// ]
// }
//
// hostname1.example.com,1.2.3.4
// hostname2,2001:db8::1
//
// In other words, two fields: the first containing a host name (fully qualified
// or not), and the second containin the IP address of the target (this field is
// optional).
package targetlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name defined the name of the plugin
var (
	Name = "TargetList"
)

// AcquireParameters contains the parameters necessary to acquire targets.
type AcquireParameters struct {
	Targets []*target.Target
}

// ReleaseParameters contains the parameters necessary to release targets.
type ReleaseParameters struct {
}

// TargetList implements the contest.TargetManager interface.
type TargetList struct {
}

// ValidateAcquireParameters valides parameters that will be passed to Acquire.
func (t TargetList) ValidateAcquireParameters(params []byte) (interface{}, error) {
	var ap AcquireParameters
	if err := json.Unmarshal(params, &ap); err != nil {
		return nil, err
	}
	for _, target := range ap.Targets {
		if strings.TrimSpace(target.ID) == "" {
			return nil, errors.New("invalid target with empty ID")
		}
	}
	return ap, nil
}

// ValidateReleaseParameters valides parameters that will be passed to Release.
func (t TargetList) ValidateReleaseParameters(params []byte) (interface{}, error) {
	var rp ReleaseParameters
	if err := json.Unmarshal(params, &rp); err != nil {
		return nil, err
	}
	return rp, nil
}

// Acquire implements contest.TargetManager.Acquire
func (t *TargetList) Acquire(ctx xcontext.Context, jobID types.JobID, jobTargetManagerAcquireTimeout time.Duration, parameters interface{}, tl target.Locker) ([]*target.Target, error) {
	acquireParameters, ok := parameters.(AcquireParameters)
	if !ok {
		return nil, fmt.Errorf("Acquire expects %T object, got %T", acquireParameters, parameters)
	}

	if err := tl.Lock(ctx, jobID, jobTargetManagerAcquireTimeout, acquireParameters.Targets); err != nil {
		ctx.Warnf("Failed to lock %d targets: %v", len(acquireParameters.Targets), err)
		return nil, err
	}

	ctx.Infof("Acquired %d targets", len(acquireParameters.Targets))
	return acquireParameters.Targets, nil
}

// Release releases the acquired resources.
func (t *TargetList) Release(ctx xcontext.Context, jobID types.JobID, targets []*target.Target, params interface{}) error {
	ctx.Infof("Released %d targets", len(targets))
	return nil
}

// New builds a new TargetList object.
func New() target.TargetManager {
	return &TargetList{}
}

// Load returns the name and factory which are needed to register the
// TargetManager.
func Load() (string, target.TargetManagerFactory) {
	return Name, New
}
