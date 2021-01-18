// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/config"
)

// storage defines the storage engine used by ConTest. It can be overridden
// via the exported function SetStorage.
var storage Storage

// Storage defines the interface that storage engines must implement
type Storage interface {
	JobStorage
	EventStorage

	// Version returns the version of the storage being used
	Version() (uint64, error)
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
	Storage
	Reset() error
}

// SetStorage sets the desired storage engine for events. Switching to a new
// storage engine implies garbage collecting the old one, with possible loss of
// pending events if not flushed correctly
func SetStorage(storageEngine Storage) error {
	if storageEngine == nil {
		return fmt.Errorf("cannot configure a nil storage engine")
	}
	v, err := storageEngine.Version()
	if err != nil {
		return fmt.Errorf("could not determine storage version: %w", err)
	}
	if v < config.MinStorageVersion {
		return fmt.Errorf("could not configure storage of type %T (minimum storage version: %d, current storage version: %d)", storageEngine, config.MinStorageVersion, v)
	}
	storage = storageEngine
	return nil
}
