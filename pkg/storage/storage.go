// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package storage

import (
	"fmt"

	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// storage defines the storage engine used by ConTest. It can be overridden
// via the exported function SetStorage.
var storage Storage
var storageAsync Storage

// ConsistencyModel hints at whether queries should go to the primary database
// or any available replica (in which case, the guarantee is eventual consistency)
type ConsistencyModel int

const (
	ConsistentReadAfterWrite ConsistencyModel = iota
	ConsistentEventually
)

const consistencyModelKey = "storage_consistency_model"

// Storage defines the interface that storage engines must implement
type Storage interface {
	JobStorage
	EventStorage

	// Close flushes and releases resources associated with the storage engine.
	Close() error

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
	if storageEngine != nil {
		v, err := storageEngine.Version()
		if err != nil {
			return fmt.Errorf("could not determine storage version: %w", err)
		}
		if v < config.MinStorageVersion {
			return fmt.Errorf("could not configure storage of type %T (minimum storage version: %d, current storage version: %d)", storageEngine, config.MinStorageVersion, v)
		}
	}
	storage = storageEngine
	return nil
}

// GetStorage returns the primary storage for events.
func GetStorage() (Storage, error) {
	if storage == nil {
		return nil, fmt.Errorf("no storage engine assigned")
	}
	return storage, nil
}

// SetAsyncStorage sets the desired storage engine for read-only events. Switching to a new
// storage engine implies garbage collecting the old one, with possible loss of
// pending events if not flushed correctly
func SetAsyncStorage(storageEngine Storage) error {
	if storageEngine != nil {
		v, err := storageEngine.Version()
		if err != nil {
			return fmt.Errorf("could not determine storage version: %w", err)
		}
		if v < config.MinStorageVersion {
			return fmt.Errorf("could not configure storage of type %T (minimum storage version: %d, current storage version: %d)", storageEngine, config.MinStorageVersion, v)
		}
	}
	storageAsync = storageEngine
	return nil
}

func isStronglyConsistent(ctx xcontext.Context) bool {
	value := ctx.Value(consistencyModelKey)
	ctx.Debugf("consistency model check: %v", value)

	switch model := value.(type) {
	case ConsistencyModel:
		return model == ConsistentReadAfterWrite

	default:
		return true
	}
}

func WithConsistencyModel(ctx xcontext.Context, model ConsistencyModel) xcontext.Context {
	return xcontext.WithValue(ctx, consistencyModelKey, model)
}
