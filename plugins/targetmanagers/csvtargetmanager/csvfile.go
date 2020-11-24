// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package csvtargetmanager implements a simple target manager that parses a CSV
// file. The format of the CSV file is the following:
//
// 123,hostname1.example.com,1.2.3.4,
// 456,hostname2,,2001:db8::1
//
// In other words, four fields: the first containing a unique ID for the device
// (might be identical to the IP or FQDN), next one is FQDN,
// and then IPv4 and IPv6.
// All fields except ID are optional, but many plugins require FQDN or IP fields to
// reach the targets over the network.
package csvtargetmanager

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
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
// from a text file. Each input record looks like this: ID,FQDN,IPv4,IPv6. Only ID is required
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
		if len(record) != 4 {
			return nil, errors.New("malformed input file, need 4 entries per record (ID, FQDN, IPv4, IPv6)")
		}
		t := &target.Target{ID: strings.TrimSpace(record[0])}
		if t.ID == "" {
			return nil, errors.New("invalid empty string for host ID")
		}
		fqdn := strings.TrimSpace(record[1])
		if fqdn != "" {
			// no validation on fqdns
			t.FQDN = fqdn
		}
		if ipv4 := strings.TrimSpace(record[2]); ipv4 != "" {
			t.PrimaryIPv4 = net.ParseIP(ipv4)
			if t.PrimaryIPv4 == nil || t.PrimaryIPv4.To4() == nil {
				// didn't parse
				return nil, fmt.Errorf("invalid non-empty IPv4 address \"%s\"", ipv4)
			}
		}
		if ipv6 := strings.TrimSpace(record[3]); ipv6 != "" {
			t.PrimaryIPv6 = net.ParseIP(ipv6)
			if t.PrimaryIPv6 == nil || t.PrimaryIPv6.To16() == nil {
				// didn't parse
				return nil, fmt.Errorf("invalid non-empty IPv6 address \"%s\"", ipv6)
			}
		}
		if len(acquireParameters.HostPrefixes) == 0 {
			hosts = append(hosts, t)
		} else if t.FQDN != "" {
			// host prefix filtering only works on devices with a FQDN
			firstComponent := strings.Split(t.FQDN, ".")[0]
			for _, hp := range acquireParameters.HostPrefixes {
				if strings.HasPrefix(firstComponent, hp) {
					hosts = append(hosts, t)
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
	if err != nil {
		return nil, fmt.Errorf("failed to lock targets: %w", err)
	}
	locked, err := target.FilterTargets(lockedString, hosts)
	if err != nil {
		return nil, fmt.Errorf("can not find locked targets in hosts")
	}

	// check if we got enough
	if len(locked) >= int(acquireParameters.MinNumberDevices) {
		// done, we got enough and they are locked
	} else {
		// not enough, unlock what we got and fail
		if len(locked) > 0 {
			err = tl.Unlock(jobID, locked)
			if err != nil {
				return nil, fmt.Errorf("can't unlock targets")
			}
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
