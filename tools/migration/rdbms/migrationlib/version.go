// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migrationlib

import (
	"fmt"

	"database/sql"
	"github.com/go-sql-driver/mysql"
	"github.com/pressly/goose"
)

// queryExecutor represents the MCD interface between sql.Db and sql.Tx
// objects that is sufficient to retrieve the db version number.
type queryExecutor interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// DBVersion returns the current version of the database schema.
// The library exposes a GetDBVersion function which however requires
// a *sql.Db object. Several contest flows work instead within a
// transactional context, so we need to be able to extract the version
// in a way that is agnostic to the `sql` object used (either `sql.Tx` or
// `sql.Db`).
func DBVersion(db queryExecutor) (uint64, error) {

	rows, err := db.Query(fmt.Sprintf("select version_id, is_applied from %s order by id desc", goose.TableName()))
	if err != nil {
		if mysqlErr, ok := err.(*mysql.MySQLError); ok {
			if mysqlErr.Number == 1146 {
				// db versioning table does not exist, assume version 0
				return 0, nil
			}
		}
		return 0, fmt.Errorf("could not retrieve db version, error %+v, %T: %w", err, err, err)
	}
	defer rows.Close()

	toSkip := make([]uint64, 0)

	for rows.Next() {
		var (
			versionID uint64
			isApplied bool
		)
		if err = rows.Scan(&versionID, &isApplied); err != nil {
			return 0, fmt.Errorf("could not scan row: %w", err)
		}

		// the is_applied field tracks whether the migration was an
		// up or down migration. If we see a down migration, while
		// going through the records in descending order, then we
		// should skip that version altogether.
		skip := false
		for _, v := range toSkip {
			if v == versionID {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		if isApplied {
			return versionID, nil
		}

		// latest version of migration has not been applied.
		toSkip = append(toSkip, versionID)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("could not retrieve db version: %w", err)
	}

	// if we couldn't figure out the db version, assume we are working on version 0
	// i.e. the db is not versioned yet.
	return 0, nil
}
