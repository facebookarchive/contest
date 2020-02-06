// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

// storage defines the events storage engine used by ConTest. It can be overridden
// via the exported function SetStorage and it can be retrieved via the exported
// function GetStorage
var storage Storage

// Storage defines the interface that storage engines must implement
type Storage interface {
	// Test events storage interface
	StoreTestEvent(event testevent.Event) error
	GetTestEvents(eventQuery *testevent.Query) ([]testevent.Event, error)

	// Framework events storage interface
	StoreFrameworkEvent(event frameworkevent.Event) error
	GetFrameworkEvent(eventQuery *frameworkevent.Query) ([]frameworkevent.Event, error)

	// Job request interface
	StoreJobRequest(request *job.Request) (types.JobID, error)
	GetJobRequest(jobID types.JobID) (*job.Request, error)

	// Job report interface
	StoreJobReport(report *job.Report) error
	GetJobReport(jobID types.JobID) (*job.Report, error)

	// Reset clears the state of the storage layer
	Reset() error
}

// SetStorage sets the desired storage engine for events. Switching to a new
// storage engine implies garbage collecting the old one, with possible loss of
// pending events if not flushed correctly
func SetStorage(storageEngine Storage) {
	storage = storageEngine
}
