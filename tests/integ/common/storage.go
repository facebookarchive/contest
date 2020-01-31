// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration_storage

package common

import (
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/plugins/storage/rdbms"
)

func NewStorage(opts ...rdbms.Opt) storage.Storage {
	dbURI := "contest:contest@tcp(mysql:3306)/contest_integ?parseTime=true"
	return rdbms.New(dbURI, opts...)
}
