// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package dblocker

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/benbjohnson/clock"
	// this blank import registers the mysql driver
	_ "github.com/go-sql-driver/mysql"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the plugin name.
var Name = "DBLocker"

const DefaultMaxBatchSize = 100

// dblock represents parts of lock in the database, basically
// a row from SELECT target_id, job_ID, expires_at
type dblock struct {
	targetID  string
	jobID     int64
	createdAt time.Time
	expiresAt time.Time
}

// String pretty-prints dblocks for logging and errors
func (d dblock) String() string {
	return fmt.Sprintf(
		"target: %s job: %d created: %s expires: %s",
		d.targetID, d.jobID, d.createdAt, d.expiresAt,
	)
}

// targetIDList is a helper to convert contest targets to
// a list of primary IDs used in the database
func targetIDList(targets []*target.Target) []string {
	res := make([]string, 0, len(targets))
	for _, target := range targets {
		res = append(res, target.ID)
	}
	return res
}

// listQueryString is a helper to create a (?, ?, ?) string
// with as many ? as requested.
// This can safely be concatenated into SQL queries as it
// can never contain input data, it only repeates a static
// string a given number of times.
func listQueryString(length uint) string {
	switch length {
	case 0:
		return "()"
	case 1:
		return "(?)"
	default:
		return "(" + strings.Repeat("?, ", int(length)-1) + "?)"
	}
}

// DBLocker implements a simple target locker based on a relational database.
// The current implementation only supports MySQL officially.
// All functions in DBLocker are safe for concurrent use by multiple goroutines.
type DBLocker struct {
	driverName   string
	db           *sql.DB
	maxBatchSize int
	// clock is used for measuring time
	clock clock.Clock
}

// queryLocks returns a map of ID -> dblock for a given list of targets
func (d *DBLocker) queryLocks(tx *sql.Tx, targets []string) (map[string]dblock, error) {
	q := "SELECT target_id, job_id, created_at, expires_at FROM locks WHERE target_id IN " + listQueryString(uint(len(targets)))
	// convert targets to a list of interface{}
	queryList := make([]interface{}, 0, len(targets))
	for _, targetID := range targets {
		queryList = append(queryList, targetID)
	}

	rows, err := tx.Query(q, queryList...)
	if err != nil {
		return nil, fmt.Errorf("unable to read existing locks: %w", err)
	}
	defer rows.Close()

	row := dblock{}
	locks := make(map[string]dblock)
	for rows.Next() {
		if err := rows.Scan(&row.targetID, &row.jobID, &row.createdAt, &row.expiresAt); err != nil {
			return nil, fmt.Errorf("unexpected read from database: %w", err)
		}
		locks[row.targetID] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unexpected error iterating db read results: %w", err)
	}
	return locks, nil
}

// handleLock does the real locking, it assumes the jobID is valid
func (d *DBLocker) handleLock(ctx xcontext.Context, jobID int64, targets []string, limit uint, timeout time.Duration, requireLocked bool, allowConflicts bool) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}

	// everything operates on this frozen time
	now := d.clock.Now()
	expiresAt := now.Add(timeout)

	tx, err := d.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("unable to start database transaction: %w", err)
	}
	defer func() {
		// this always fails if tx.Commit() was called before, ignore error
		_ = tx.Rollback()
	}()

	locks, err := d.queryLocks(tx, targets)
	if err != nil {
		return nil, err
	}
	// go through existing locks, they are either held by something else and valid
	// (abort or skip, depending on allowConflicts setting),
	// not valid anymore (update), held by something else and not valid (expired),
	// held by us or not held at all (insert)
	var toInsert, missing []string
	var toDelete, conflicts []dblock
	for _, targetID := range targets {
		lock, ok := locks[targetID]
		switch {
		case !ok: // nonexistent lock
			if requireLocked {
				missing = append(missing, targetID)
			}
			toInsert = append(toInsert, targetID)
		case lock.jobID == jobID: // our lock, possibly expired
			toDelete = append(toDelete, lock)
			toInsert = append(toInsert, targetID)
		case lock.expiresAt.Before(now): // other job's expired lock
			if !requireLocked {
				toDelete = append(toDelete, lock)
				toInsert = append(toInsert, targetID)
			} else {
				conflicts = append(conflicts, lock)
			}
		default:
			conflicts = append(conflicts, lock)
		}
		if uint(len(toInsert)) >= limit {
			break
		}
	}
	if (len(conflicts) > 0 && !allowConflicts) || len(missing) > 0 {
		return nil, fmt.Errorf("unable to lock targets %v for owner %d, have %d conflicting locks (%v), %d missing locks (%v)",
			targets, jobID, len(conflicts), conflicts, len(missing), missing)
	}

	// First, drop all the locks that we intend to extend or take over.
	// Use strict matching so that if another instance races ahead of us, row will not be deleted and subsequent insert will fail.
	{
		var stmt []string
		var args []interface{}
		for i, lock := range toDelete {
			if len(stmt) == 0 {
				stmt = append(stmt, "DELETE FROM locks WHERE (target_id = ? AND job_id = ? AND expires_at = ?)")
			} else {
				stmt = append(stmt, " OR (target_id = ? AND job_id = ? AND expires_at = ?)")
			}
			args = append(args, lock.targetID, lock.jobID, lock.expiresAt)
			if len(stmt) < d.maxBatchSize && i < len(toDelete)-1 {
				continue
			}
			if _, err := tx.Exec(strings.Join(stmt, ""), args...); err != nil {
				return nil, fmt.Errorf("insert statement failed: %w", err)
			}
			stmt = nil
			args = nil
		}
	}

	// Now insert new entries for all the targets we are locking.
	{
		var stmt []string
		var args []interface{}
		for i, targetID := range toInsert {
			createdAt := now
			// If we are updating our own lock, carry over the creation timestamp.
			if lock, ok := locks[targetID]; ok && lock.jobID == jobID {
				createdAt = lock.createdAt
			}
			if len(stmt) == 0 {
				stmt = append(stmt, "INSERT INTO locks (target_id, job_id, created_at, expires_at, valid) VALUES (?, ?, ?, ?, ?)")
			} else {
				stmt = append(stmt, ", (?, ?, ?, ?, ?)")
			}
			args = append(args, targetID, jobID, createdAt, expiresAt, true)
			if len(stmt) < d.maxBatchSize && i < len(toInsert)-1 {
				continue
			}
			if _, err := tx.Exec(strings.Join(stmt, ""), args...); err != nil {
				return nil, fmt.Errorf("insert statement failed: %w", err)
			}
			stmt = nil
			args = nil
		}
	}

	return toInsert, tx.Commit()
}

// handleUnlock does the real unlocking, it assumes the jobID is valid
func (d *DBLocker) handleUnlock(ctx xcontext.Context, jobID int64, targets []string) error {
	if len(targets) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("unable to start database transaction: %w", err)
	}
	defer func() {
		// this always fails if tx.Commit() was called before, ignore error
		_ = tx.Rollback()
	}()

	// check lock states: must own locks for all targets (expired is ok too).
	locks, err := d.queryLocks(tx, targets)
	if err != nil {
		return err
	}
	for _, t := range targets {
		l, ok := locks[t]
		if !ok {
			return fmt.Errorf("target %q is not locked", t)
		}
		if l.jobID != jobID {
			return fmt.Errorf("unlock request: target %q is locked by %q, not by %q", t, l.jobID, jobID)
		}
	}

	// drop non-conflicting locks
	del := "DELETE FROM locks WHERE job_id = ? AND target_id IN " + listQueryString(uint(len(targets)))
	queryList := make([]interface{}, 0, len(targets)+1)
	queryList = append(queryList, jobID)
	for _, targetID := range targets {
		queryList = append(queryList, targetID)
	}
	if _, err := tx.Exec(del, queryList...); err != nil {
		return fmt.Errorf("unable to unlock targets %v, owner %d: %w", targets, jobID, err)
	}
	return tx.Commit()
}

func validateTargets(targets []*target.Target) error {
	for _, target := range targets {
		if target.ID == "" {
			return fmt.Errorf("target list cannot contain empty target ID. Full list: %v", targets)
		}
	}
	return nil
}

// Lock locks the given targets.
// See target.Locker for API details
func (d *DBLocker) Lock(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*target.Target) error {
	if jobID == 0 {
		return fmt.Errorf("invalid lock request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return fmt.Errorf("invalid lock request: %w", err)
	}
	_, err := d.handleLock(ctx, int64(jobID), targetIDList(targets), uint(len(targets)), duration, false /* requireLocked */, false /* allowConflicts */)
	ctx.Debugf("Lock %d targets for %s: %v", len(targets), duration, err)
	return err
}

// TryLock attempts to locks the given targets.
// See target.Locker for API details
func (d *DBLocker) TryLock(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*target.Target, limit uint) ([]string, error) {
	if jobID == 0 {
		return nil, fmt.Errorf("invalid tryLock request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return nil, fmt.Errorf("invalid tryLock request: %w", err)
	}
	if limit == 0 {
		return nil, nil
	}
	res, err := d.handleLock(ctx, int64(jobID), targetIDList(targets), limit, duration, false /* requireLocked */, true /* allowConflicts */)
	ctx.Debugf("TryLock %d targets for %s: %d %v", len(targets), duration, len(res), err)
	return res, err
}

// Unlock unlocks the given targets.
// See target.Locker for API details
func (d *DBLocker) Unlock(ctx xcontext.Context, jobID types.JobID, targets []*target.Target) error {
	if jobID == 0 {
		return fmt.Errorf("invalid unlock request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return fmt.Errorf("invalid unlock request: %w", err)
	}
	err := d.handleUnlock(ctx, int64(jobID), targetIDList(targets))
	ctx.Debugf("Unlock %d targets: %v", len(targets), err)
	return err
}

// RefreshLocks refreshes the locks on the given targets.
// See target.Locker for API details
func (d *DBLocker) RefreshLocks(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*target.Target) error {
	if jobID == 0 {
		return fmt.Errorf("invalid refresh request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return fmt.Errorf("invalid refresh request: %w", err)
	}
	_, err := d.handleLock(ctx, int64(jobID), targetIDList(targets), uint(len(targets)), duration, true /* requireLocked */, false /* allowConflicts */)
	ctx.Debugf("RefreshLocks on %d targets for %s: %v", len(targets), duration, err)
	return err
}

// Close closes the DB connection and releases resources.
func (d *DBLocker) Close() error {
	return d.db.Close()
}

// ResetAllLocks resets the database and clears all locks, regardless of who owns them.
// This is primarily for testing, and should not be used by used in prod, this
// is why it is not exposed by target.Locker
func (d *DBLocker) ResetAllLocks(ctx xcontext.Context) error {
	ctx.Warnf("DELETING ALL LOCKS")
	_, err := d.db.Exec("TRUNCATE TABLE locks")
	return err
}

// Opt is a function type that sets parameters on the DBLocker object
type Opt func(dblocker *DBLocker)

// WithDriverName option allows using a mysql-compatible driver (e.g. a wrapper around mysql
// or a syntax-compatible variant).
func WithDriverName(name string) Opt {
	return func(d *DBLocker) {
		d.driverName = name
	}
}

// WithMaxBatchSize option sets maximum batch size for statements.
func WithMaxBatchSize(value int) Opt {
	return func(d *DBLocker) {
		d.maxBatchSize = value
	}
}

// WithClock option sets clock used for timestamps.
func WithClock(value clock.Clock) Opt {
	return func(d *DBLocker) {
		d.clock = value
	}
}

// New initializes and returns a new DBLocker target locker.
func New(dbURI string, opts ...Opt) (*DBLocker, error) {
	res := &DBLocker{
		maxBatchSize: DefaultMaxBatchSize,
		clock:        clock.New(),
	}

	for _, Opt := range opts {
		Opt(res)
	}

	driverName := "mysql"
	if res.driverName != "" {
		driverName = res.driverName
	}
	db, err := sql.Open(driverName, dbURI)
	if err != nil {
		return nil, fmt.Errorf("could not initialize database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("unable to contact database: %w", err)
	}
	res.db = db
	return res, nil
}
