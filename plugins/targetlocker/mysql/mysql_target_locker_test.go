// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package mysql

import (
	"database/sql"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	. "github.com/facebookincubator/contest/plugins/targetlocker"
)

func init() {
	enableDebug = true
}

func mockInstance(t *testing.T) (target.Locker, *sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	returnRows := sqlmock.NewRows([]string{"target_id", "job_id", "created_at", "expires_at"})
	returnRows.AddRow("z", -1, time.Now(), time.Now())
	mock.ExpectQuery("SELECT . FROM `locks` LIMIT 1").WillReturnRows(returnRows).RowsWillBeClosed()

	tl, err := (&Factory{}).new(time.Hour, db)
	require.NoError(t, err)
	require.NotNil(t, tl)
	require.IsType(t, &TargetLocker{}, tl)

	return tl, db, mock
}

type sqlResult struct {
	rowsAffected uint
	error        error
}

func (res *sqlResult) RowsAffected() (int64, error) {
	return int64(res.rowsAffected), res.error
}
func (res *sqlResult) LastInsertId() (int64, error) {
	panic("not implemented")
}

func TestMySQLTargetLocker_Lock_positive(t *testing.T) {
	tl, db, mock := mockInstance(t)
	defer db.Close()

	mock.ExpectBegin()
	qDeleteRegExp := "DELETE FROM `locks` WHERE target_id=. AND NOW.* >= expires_at"
	qAddRegExp := "INSERT INTO `locks` SET target_id=., job_id=., created_at = NOW.., expires_at = NOW.. [+] INTERVAL . SECOND"
	mock.ExpectExec(qDeleteRegExp).WithArgs("a").WillReturnResult(&sqlResult{0, sql.ErrNoRows})
	mock.ExpectExec(qAddRegExp).WithArgs("a", 1, 3600).WillReturnResult(&sqlResult{1, nil})
	mock.ExpectExec(qDeleteRegExp).WithArgs("b").WillReturnResult(&sqlResult{0, sql.ErrNoRows})
	mock.ExpectExec(qAddRegExp).WithArgs("b", 1, 3600).WillReturnResult(&sqlResult{1, nil})
	mock.ExpectCommit()

	err := tl.Lock(types.JobID(1), []*target.Target{{ID: "a"}, {ID: "b"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLTargetLocker_Lock_negative(t *testing.T) {
	tl, db, mock := mockInstance(t)
	defer db.Close()

	mock.ExpectBegin()
	qDeleteRegExp := "DELETE FROM `locks` WHERE target_id=. AND NOW.* >= expires_at"
	qAddRegExp := "INSERT INTO `locks` SET target_id=., job_id=., created_at = NOW.., expires_at = NOW.. [+] INTERVAL . SECOND"
	mock.ExpectExec(qDeleteRegExp).WithArgs("a").WillReturnResult(&sqlResult{0, sql.ErrNoRows})
	mock.ExpectExec(qAddRegExp).WithArgs("a", 1, 3600).WillReturnResult(&sqlResult{1, nil})
	mock.ExpectExec(qDeleteRegExp).WithArgs("a").WillReturnResult(&sqlResult{0, sql.ErrNoRows})
	mock.ExpectExec(qAddRegExp).WithArgs("a", 1, 3600).WillReturnResult(
		&sqlResult{rowsAffected: 0,
			error: &mysql.MySQLError{
				Number:  1062,
				Message: "Duplicate primary key (unit-test)",
			}})
	mock.ExpectRollback()

	err := tl.Lock(types.JobID(1), []*target.Target{{ID: "a"}, {ID: "a"}})
	require.Error(t, err)
	require.True(t, xerrors.As(err, &ErrAlreadyLocked{}), err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLTargetLocker_Unlock(t *testing.T) {
	tl, db, mock := mockInstance(t)
	defer db.Close()

	mock.ExpectBegin()
	qRegExp := "DELETE FROM `locks` WHERE target_id=. AND job_id=?"
	mock.ExpectExec(qRegExp).WithArgs("a", 1).WillReturnResult(&sqlResult{1, nil})
	mock.ExpectCommit()

	err := tl.Unlock(types.JobID(1), []*target.Target{{ID: "a"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLTargetLocker_RefreshLocks_positive(t *testing.T) {
	tl, db, mock := mockInstance(t)
	defer db.Close()

	mock.ExpectBegin()
	qRegExp := "UPDATE `locks` SET expires_at = NOW.. . INTERVAL . SECOND WHERE target_id=. AND job_id=."
	mock.ExpectExec(qRegExp).
		WithArgs(3600, "a", 1).WillReturnResult(&sqlResult{1, nil})
	mock.ExpectCommit()

	err := tl.RefreshLocks(types.JobID(1), []*target.Target{{ID: "a"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLTargetLocker_CheckLocks_positive(t *testing.T) {
	tl, db, mock := mockInstance(t)
	defer db.Close()

	allTargets := []*target.Target{{ID: "c"}, {ID: "a"}, {ID: "b"}, {ID: "d"}}

	mockLockedTargetIDs := []string{"b", "c"}
	returnRows := sqlmock.NewRows([]string{"target_id"})
	for _, targetID := range mockLockedTargetIDs {
		returnRows.AddRow(targetID)
	}

	var mockNotLockedTargetIDs []string
	for _, targetItem := range allTargets {
		isLocked := false
		for _, lockedTargetID := range mockLockedTargetIDs {
			if targetItem.ID == lockedTargetID {
				isLocked = true
				break
			}
		}
		if isLocked {
			continue
		}
		mockNotLockedTargetIDs = append(mockNotLockedTargetIDs, targetItem.ID)
	}

	qRegExp := "SELECT `target_id` FROM `locks` WHERE job_id=. AND target_id IN \\((\\?(,\\?)*)?\\)"
	mock.ExpectQuery(qRegExp).
		WithArgs(1, "a", "b", "c", "d").WillReturnRows(returnRows)

	locked, notLocked, err := tl.CheckLocks(types.JobID(1), allTargets)

	sort.Strings(mockLockedTargetIDs)
	sort.Strings(mockNotLockedTargetIDs)

	require.NoError(t, err)
	require.Equal(t, len(allTargets), len(locked)+len(notLocked))
	require.Equal(t, len(mockLockedTargetIDs), len(locked))
	for idx, lockedTargetID := range mockLockedTargetIDs {
		require.Equal(t, lockedTargetID, locked[idx].ID)
	}
	for idx, notLockedTargetID := range mockNotLockedTargetIDs {
		require.Equal(t, notLockedTargetID, notLocked[idx].ID)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
