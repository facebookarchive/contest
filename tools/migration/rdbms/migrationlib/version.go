// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migrationlib

import (
	"database/sql"
	"github.com/pressly/goose"
)

// DBVersion returns the current version of the database schema
func DBVersion(db *sql.DB) (int64, error) {
	return goose.GetDBVersion(db)
}
