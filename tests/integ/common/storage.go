// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package common

import (
	"os"

	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
)

func GetDatabaseURI() string {
	if os.Getenv("CI") != "" {
		return "contest:contest@tcp(mysql:3306)/contest_integ?parseTime=true"
	} else {
		return "contest:contest@tcp(localhost:3306)/contest_integ?parseTime=true"
	}
}

func NewStorage(opts ...rdbms.Opt) (storage.Storage, error) {
	return rdbms.New(GetDatabaseURI(), opts...)
}

// InitStorage initializes the storage backend with a new transaction, if supported
func InitStorage(s storage.Storage) storage.Storage {
	switch s := s.(type) {
	case storage.ResettableStorage:
		_ = s.Reset()
	default:
		panic("unknown storage type")
	}
	return s
}

// FinalizeStorage finalizes the storage layer with either a rollback of the current transaction
// or by resetting altogether the backend, if supported.
func FinalizeStorage(s storage.Storage) {
	switch s := s.(type) {
	case storage.ResettableStorage:
		_ = s.Reset()
	default:
		panic("unknown storage type")
	}
}
