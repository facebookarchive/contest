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

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// Name defined the name of the plugin
var (
	Name = "TargetList"
)

var log = logging.GetLogger("targetmanagers/" + strings.ToLower(Name))

// AcquireParameters contains the parameters necessary to acquire targets.
type AcquireParameters struct {
	Targets []*target.Target
}

// ReleaseParameters contains the parameters necessary to release targets.
type ReleaseParameters struct {
}

// TargetList implements the contest.TargetManager interface.
type TargetList struct {
	targets []*target.Target
}

// ValidateAcquireParameters performs sanity checks on the fields of the
// parameters that will be passed to Acquire.
func (t TargetList) ValidateAcquireParameters(params []byte) (interface{}, error) {
	var ap AcquireParameters
	if err := json.Unmarshal(params, &ap); err != nil {
		return nil, err
	}
	for _, target := range ap.Targets {
		if strings.TrimSpace(target.Name) == "" {
			return nil, errors.New("invalid target with empty name")
		}
	}
	return ap, nil
}

// ValidateReleaseParameters performs sanity checks on the fields of the
// parameters that will be passed to Release.
func (t TargetList) ValidateReleaseParameters(params []byte) (interface{}, error) {
	var rp ReleaseParameters
	if err := json.Unmarshal(params, &rp); err != nil {
		return nil, err
	}
	return rp, nil
}

// Acquire implements contest.TargetManager.Acquire, reading one entry per line
// from a text file. Each input record has a hostname, a space, and a host ID.
func (t *TargetList) Acquire(jobID types.JobID, cancel <-chan struct{}, parameters interface{}, tl target.Locker) ([]*target.Target, error) {
	acquireParameters, ok := parameters.(AcquireParameters)
	if !ok {
		return nil, fmt.Errorf("Acquire expects %T object, got %T", acquireParameters, parameters)
	}

	if err := tl.Lock(jobID, acquireParameters.Targets); err != nil {
		log.Warningf("Failed to lock %d targets: %v", len(acquireParameters.Targets), err)
		return nil, err
	}
	t.targets = acquireParameters.Targets
	log.Infof("Acquired %d targets", len(t.targets))
	return acquireParameters.Targets, nil
}

// Release releases the acquired resources.
func (t *TargetList) Release(jobID types.JobID, cancel <-chan struct{}, params interface{}) error {
	log.Infof("Released %d targets", len(t.targets))
	return nil
}

// Factory implements target.TargetManagerFactory
type Factory struct{}

// New builds a TargetList
func (f *Factory) New() target.TargetManager {
	return &TargetList{}
}

// UniqueImplementationName returns the unique name of the implementation
func (f *Factory) UniqueImplementationName() string {
	return Name
}
