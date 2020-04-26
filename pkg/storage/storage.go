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

// storage defines the storage engine used by ConTest. It can be overridden
// via the exported function SetStorage.
var storage Storage

// JobStorage defines the interface that implements persistence for job
// related information
type JobStorage interface {
	// Job request interface
	StoreJobRequest(request *job.Request) (types.JobID, error)
	GetJobRequest(jobID types.JobID) (*job.Request, error)

	// Job report interface
	StoreJobReport(report *job.JobReport) error
	GetJobReport(jobID types.JobID) (*job.JobReport, error)
}

// Storage defines the interface that storage engines must implement
type Storage interface {
	JobStorage

	// Test events storage interface
	StoreTestEvent(event testevent.Event) error
	GetTestEvents(eventQuery *testevent.Query) ([]testevent.Event, error)

	// Framework events storage interface
	StoreFrameworkEvent(event frameworkevent.Event) error
	GetFrameworkEvent(eventQuery *frameworkevent.Query) ([]frameworkevent.Event, error)
}

// TransactionalStorage is implemented by storage backends that support transactions.
// Only default isolation level is supported.
type TransactionalStorage interface {
	Storage
	BeginTx() (TransactionalStorage, error)
	Commit() error
	Rollback() error
}

// ResettableStorage is implemented by storage engines that support reset operation
type ResettableStorage interface {
	Reset() error
}

// SetStorage sets the desired storage engine for events. Switching to a new
// storage engine implies garbage collecting the old one, with possible loss of
// pending events if not flushed correctly
func SetStorage(storageEngine Storage) {
	storage = storageEngine
}
