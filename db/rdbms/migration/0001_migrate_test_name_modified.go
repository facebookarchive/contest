// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migration

import (
	"database/sql"
	"fmt"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/tools/migration"
)

// TestNameMigration implements a migration of test_name column
type TestNameMigration struct {
}

// Desc returns the description of the migration task
func (m TestNameMigration) Desc() string {
	return "add test_name_modified to test_events"
}

// Version returns the version of the migration that TestNameMigration implements
func (m TestNameMigration) Version() uint {
	return 1
}

// Count returns the number of records that the task should migrate
func (m TestNameMigration) Count(db *sql.DB) (uint, error) {
	count := uint(0)

	rows, err := db.Query("select count(*) from test_events")
	if err != nil {
		return 0, fmt.Errorf("could not fetch number of records to migrate: %v", err)
	}
	if rows.Next() == false {
		return 0, fmt.Errorf("could not fetch number of records to migrate, at least one result from count(*) expected")
	}

	if err := rows.Scan(&count); err != nil {
		return 0, fmt.Errorf("could not fetch number of records to migrate: %v", err)
	}

	return count, nil
}

// Up returns the upward schema migration instructions
func (m TestNameMigration) Up() string {
	return "ALTER TABLE test_events ADD COLUMN `test_name_modified` VARCHAR(64);"
}

// Down returns the upward schema migration instructions
func (m TestNameMigration) Down() string {
	return ""
}

// MigrateData runs the data migration task
func (m TestNameMigration) MigrateData(db *sql.DB, terminate chan struct{}, progress chan *migration.Progress) error {

	count := uint64(0)
	rows, err := db.Query("select count(*) from test_events")
	if err != nil {
		return fmt.Errorf("could not fetch number of records to migrate: %v", err)
	}
	if rows.Next() == false {
		return fmt.Errorf("could not fetch number of records to migrate, at least one result from count(*) expected")
	}

	if err := rows.Scan(&count); err != nil {
		return fmt.Errorf("could not fetch number of records to migrate: %v", err)
	}

	p := migration.Progress{Total: count}

	query := "select event_id, job_id, run_id, test_name, test_step_label, event_name, target_name, target_id, payload, emit_time from test_events"
	rows, err = db.Query(query)
	if err != nil {
		return fmt.Errorf("could not fetch all test events to migrate: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	// TargetName and TargetID might be null, so a type which supports null should be used with Scan
	var (
		targetName sql.NullString
		targetID   sql.NullString
		payload    sql.NullString
	)

	for rows.Next() {
		data := testevent.Data{}
		header := testevent.Header{}
		event := testevent.New(&header, &data)

		var eventID int
		err := rows.Scan(
			&eventID,
			&header.JobID,
			&header.RunID,
			&header.TestName,
			&header.TestStepLabel,
			&data.EventName,
			&targetName,
			&targetID,
			&payload,
			&event.EmitTime,
		)
		if err != nil {
			return fmt.Errorf("could not scan row: %+v", err)
		}

		testNameModified := fmt.Sprintf("%s_%d", header.TestName, eventID)
		query := "update test_events set test_name_modified = ? where event_id = ?"
		_, err = db.Exec(query, testNameModified, eventID)
		if err != nil {
			return fmt.Errorf("could not modify record on event id %d: %+v", eventID, err)
		}

		p.Completed++
		if p.Completed%100 == 0 {
			progress <- &p
		}

	}

	return nil
}
