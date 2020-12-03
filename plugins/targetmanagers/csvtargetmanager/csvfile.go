// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package csvtargetmanager implements a simple target manager that parses a CSV
// file. The format of the CSV file is the following:
//
// hostname1.example.com,1.2.3.4
// hostname2,2001:db8::1
//
// In other words, two fields: the first containing a host name (fully qualified
// or not), and the second containin the IP address of the target (this field is
// optional).
package csvtargetmanager

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/insomniacslk/xjson"
)

// Name defined the name of the plugin
var (
	Name = "CSVFileTargetManager"
)

var log = logging.GetLogger("targetmanagers/" + strings.ToLower(Name))

// AcquireParameters contains the parameters necessary to acquire targets.
type AcquireParameters struct {
	FileURI          *xjson.URL
	MinNumberDevices uint32
	MaxNumberDevices uint32
	HostPrefixes     []string
	Shuffle          bool
}

// ReleaseParameters contains the parameters necessary to release targets.
type ReleaseParameters struct {
}

// CSVFileTargetManager implements the contest.TargetManager interface, reading
// CSV entries from a text file.
type CSVFileTargetManager struct {
	hosts []*target.Target
}

// ValidateAcquireParameters performs sanity checks on the fields of the
// parameters that will be passed to Acquire.
func (tf CSVFileTargetManager) ValidateAcquireParameters(params []byte) (interface{}, error) {
	var ap AcquireParameters
	if err := json.Unmarshal(params, &ap); err != nil {
		return nil, err
	}
	for idx, hp := range ap.HostPrefixes {
		hp = strings.TrimSpace(hp)
		if hp == "" {
			return nil, fmt.Errorf("Host prefix cannot be empty string if specified")
		}
		// reassign after removing surrounding spaces
		ap.HostPrefixes[idx] = hp
	}
	if ap.FileURI == nil {
		return nil, fmt.Errorf("file URI not specified in acquire parameters")
	}
	if ap.FileURI.Scheme != "file" && ap.FileURI.Scheme != "" {
		return nil, fmt.Errorf("unsupported scheme: '%s', only 'file' or empty string are accepted", ap.FileURI.Scheme)
	}
	if ap.FileURI.Host != "" && ap.FileURI.Host != "localhost" {
		return nil, fmt.Errorf("unsupported host '%s', only 'localhost' or empty string are accepted", ap.FileURI.Host)
	}
	return ap, nil
}

// ValidateReleaseParameters performs sanity checks on the fields of the
// parameters that will be passed to Release.
func (tf CSVFileTargetManager) ValidateReleaseParameters(params []byte) (interface{}, error) {
	var rp ReleaseParameters
	if err := json.Unmarshal(params, &rp); err != nil {
		return nil, err
	}
	return rp, nil
}

// Acquire implements contest.TargetManager.Acquire, reading one entry per line
// from a text file. Each input record has a hostname, a space, and a host ID.
func (tf *CSVFileTargetManager) Acquire(jobID types.JobID, cancel <-chan struct{}, parameters interface{}, tl target.Locker) ([]*target.Target, error) {
	acquireParameters, ok := parameters.(AcquireParameters)
	if !ok {
		return nil, fmt.Errorf("Acquire expects %T object, got %T", acquireParameters, parameters)
	}
	fd, err := os.Open(acquireParameters.FileURI.Path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	hosts := make([]*target.Target, 0)
	r := csv.NewReader(fd)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) == 0 {
			// skip blank lines
			continue
		}
		if len(record) != 2 {
			return nil, errors.New("malformed input file, need exactly two fields per record")
		}
		name, id := strings.TrimSpace(record[0]), strings.TrimSpace(record[1])
		if name == "" || id == "" {
			return nil, errors.New("invalid empty string for host name or host ID")
		}
		// no need to check if there is at least one item, the non-empty string
		// has been checked already and this will always return at least one
		// item.
		firstComponent := strings.Split(name, ".")[0]
		if len(acquireParameters.HostPrefixes) == 0 {
			hosts = append(hosts, &target.Target{Name: name, ID: id})
		} else {
			for _, hp := range acquireParameters.HostPrefixes {
				if strings.HasPrefix(firstComponent, hp) {
					hosts = append(hosts, &target.Target{Name: name, ID: id})
				}
			}
		}
	}

	if uint32(len(hosts)) < acquireParameters.MinNumberDevices {
		return nil, fmt.Errorf("not enough hosts found in CSV file '%s', want %d, got %d",
			acquireParameters.FileURI.Path,
			acquireParameters.MinNumberDevices,
			len(hosts),
		)
	}
	log.Printf("Found %d targets in %s", len(hosts), acquireParameters.FileURI.Path)
	if acquireParameters.Shuffle {
		log.Info("Shuffling targets")
		rand.Shuffle(len(hosts), func(i, j int) {
			hosts[i], hosts[j] = hosts[j], hosts[i]
		})
	}

	// feed all devices into new API call `TryLock`, with desired limit
	lockedString, err := tl.TryLock(jobID, hosts, uint(acquireParameters.MaxNumberDevices))
	locked, err := target.FilterTargets(lockedString, hosts)

	// check if we got enough
	if len(locked) >= int(acquireParameters.MinNumberDevices) {
		// done, we got enough and they are locked
	} else {
		// not enough, unlock what we got and fail
		if len(locked) > 0 {
			tl.Unlock(jobID, locked)
		}
		return nil, fmt.Errorf("can't lock enough targets")
	}

	tf.hosts = locked
	return locked, nil
}

// Release releases the acquired resources.
func (tf *CSVFileTargetManager) Release(jobID types.JobID, cancel <-chan struct{}, params interface{}) error {
	return nil
}

// New builds a CSVFileTargetManager
func New() target.TargetManager {
	return &CSVFileTargetManager{}
}

// Load returns the name and factory which are needed to register the
// TargetManager.
func Load() (string, target.TargetManagerFactory) {
	return Name, New
}
