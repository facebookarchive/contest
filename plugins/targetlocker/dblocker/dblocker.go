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

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"

	// this blank import registers the mysql driver
	_ "github.com/go-sql-driver/mysql"
)

// Name is the plugin name.
var Name = "DBLocker"

var log = logging.GetLogger("targetlocker/" + strings.ToLower(Name))

// dblock represents parts of lock in the database, basically
// a row from SELECT target_id, job_ID, expires_at
type dblock struct {
	targetID  string
	jobID     int64
	expiresAt time.Time
	valid     bool
}

// String pretty-prints dblocks for logging and errors
func (d dblock) String() string {
	return fmt.Sprintf("target: %s job: %d expires: %s valid: %t", d.targetID, d.jobID, d.expiresAt, d.valid)
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
	driverName string
	db         *sql.DB
	// lockTimeout set on each initial lock request
	lockTimeout time.Duration
	// refreshTimeout is used during refresh
	refreshTimeout time.Duration
}

// lockDBRows locks the rows for the given targets in the database
// to prevent later races and deadlocks
func (d *DBLocker) lockDBRows(tx *sql.Tx, targets []string) error {
	q := "SELECT * FROM locks WHERE target_id IN " + listQueryString(uint(len(targets))) + " FOR UPDATE;"
	// convert targets to a list of interface{}
	queryList := make([]interface{}, 0, len(targets))
	for _, targetID := range targets {
		queryList = append(queryList, targetID)
	}
	_, err := tx.Exec(q, queryList...)
	if err != nil {
		return fmt.Errorf("unable lock DB rows: %w", err)
	}
	return nil
}

// queryLocks returns a map of ID -> dblock for a given list of targets
func (d *DBLocker) queryLocks(tx *sql.Tx, targets []string) (map[string]dblock, error) {
	q := "SELECT target_id, job_id, expires_at, valid FROM locks WHERE target_id IN " + listQueryString(uint(len(targets))) + ";"
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
		if err := rows.Scan(&row.targetID, &row.jobID, &row.expiresAt, &row.valid); err != nil {
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
func (d *DBLocker) handleLock(jobID int64, targets []string, timeout time.Duration, allowConflicts bool) ([]string, error) {
	// everything operates on this frozen time
	now := time.Now()
	expires := now.Add(timeout)

	tx, err := d.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("unable to start database transaction: %w", err)
	}
	defer func() {
		// this always fails if tx.Commit() was called before, ignore error
		_ = tx.Rollback()
	}()

	// immediately lock db rows to prevent deadlocks with concurrent transactions
	if err := d.lockDBRows(tx, targets); err != nil {
		return nil, err
	}

	locks, err := d.queryLocks(tx, targets)
	if err != nil {
		return nil, err
	}
	// go through existing locks, they are either held by something else and valid
	// (abort or skip, depending on allowConflicts setting),
	// not valid anymore (update), held by something else and valid but expired (update),
	// held by us and valid (update), or not held at all(insert)
	inserts := make([]string, 0)
	updates := make([]string, 0)
	conflicts := make([]dblock, 0)
	for _, t := range targets {
		lock, ok := locks[t]
		switch {
		case !ok:
			inserts = append(inserts, t)
		case !lock.valid:
			updates = append(updates, t)
		case lock.jobID == jobID:
			updates = append(updates, t)
		case lock.jobID != jobID && (lock.expiresAt.Before(now) || lock.expiresAt.Equal(now)):
			updates = append(updates, t)
		default:
			conflicts = append(conflicts, lock)
		}
	}
	if len(conflicts) > 0 && !allowConflicts {
		return nil, fmt.Errorf("unable to lock targets %v for owner %d, have conflicting locks: %v", targets, jobID, conflicts)
	}

	ins := "INSERT INTO locks (target_id, job_id, created_at, expires_at, valid) VALUES (?, ?, ?, ?, ?);"
	for _, id := range inserts {
		if _, err := tx.Exec(ins, id, jobID, now, expires, true); err != nil {
			return nil, fmt.Errorf("unable to lock target %s: %w", id, err)
		}
	}

	upd := "UPDATE locks SET expires_at = ?, job_id = ?, valid = true WHERE target_id = ?;"
	for _, id := range updates {
		if _, err := tx.Exec(upd, expires, jobID, id); err != nil {
			return nil, fmt.Errorf("unable to refresh lock on target %s: %w", id, err)
		}
	}

	return append(inserts, updates...), tx.Commit()
}

// handleUnlock does the real unlocking, it assumes the jobID is valid
func (d *DBLocker) handleUnlock(jobID int64, targets []string) error {
	// everything operates on this frozen time
	now := time.Now()
	// unlocking is all or nothing in one transaction
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("unable to start database transaction: %w", err)
	}
	defer func() {
		// this always fails if tx.Commit() was called before, ignore error
		_ = tx.Rollback()
	}()

	// immediately lock db rows to prevent deadlocks with concurrent transactions
	if err := d.lockDBRows(tx, targets); err != nil {
		return err
	}

	locks, err := d.queryLocks(tx, targets)
	if err != nil {
		return err
	}

	// detect conflicts (unlock on valid foreign locks) and warn about them,
	// but don't abort.
	conflicts := make([]dblock, 0)
	invalidate := make([]string, 0)
	for _, lock := range locks {
		if lock.jobID != jobID && lock.expiresAt.After(now) && lock.valid {
			conflicts = append(conflicts, lock)
		} else {
			invalidate = append(invalidate, lock.targetID)
		}
	}
	if len(conflicts) > 0 {
		log.Warningf("unable to unlock targets %v for owner %d due to different lock owners: %+v", targets, jobID, conflicts)
	}
	if len(invalidate) > 0 {
		// invalidate non-conflicting locks
		del := "UPDATE locks SET valid = false WHERE target_id IN " + listQueryString(uint(len(invalidate))) + ";"
		queryList := make([]interface{}, 0, len(invalidate))
		for _, targetID := range invalidate {
			queryList = append(queryList, targetID)
		}
		_, err = tx.Exec(del, queryList...)
		if err != nil {
			return fmt.Errorf("unable to unlock targets %v, owner %d: %w", targets, jobID, err)
		}
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
func (d *DBLocker) Lock(jobID types.JobID, targets []*target.Target) error {
	if jobID == 0 {
		return fmt.Errorf("invalid lock request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return fmt.Errorf("invalid lock request: %w", err)
	}
	log.Debugf("Requested to lock %d targets for job ID %d: %v", len(targets), jobID, targets)
	if len(targets) == 0 {
		return nil
	}

	_, err := d.handleLock(int64(jobID), targetIDList(targets), d.lockTimeout, false)
	return err
}

// TryLock attempts to locks the given targets.
// See target.Locker for API details
func (d *DBLocker) TryLock(jobID types.JobID, targets []*target.Target) ([]string, error) {
	if jobID == 0 {
		return nil, fmt.Errorf("invalid tryLock request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return nil, fmt.Errorf("invalid tryLock request: %w", err)
	}
	log.Debugf("Requested to tryLock %d targets for job ID %d: %v", len(targets), jobID, targets)
	if len(targets) == 0 {
		return nil, nil
	}

	return d.handleLock(int64(jobID), targetIDList(targets), d.lockTimeout, true)
}

// Unlock unlocks the given targets.
// See target.Locker for API details
func (d *DBLocker) Unlock(jobID types.JobID, targets []*target.Target) error {
	if jobID == 0 {
		return fmt.Errorf("invalid unlock request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return fmt.Errorf("invalid unlock request: %w", err)
	}
	log.Debugf("Requested to unlock %d targets for job ID %d: %v", len(targets), jobID, targets)
	if len(targets) == 0 {
		return nil
	}

	return d.handleUnlock(int64(jobID), targetIDList(targets))
}

// RefreshLocks refreshes (or locks!) the given targets.
// See target.Locker for API details
func (d *DBLocker) RefreshLocks(jobID types.JobID, targets []*target.Target) error {
	if jobID == 0 {
		return fmt.Errorf("invalid refresh request, jobID cannot be zero (targets: %v)", targets)
	}
	if err := validateTargets(targets); err != nil {
		return fmt.Errorf("invalid refresh request: %w", err)
	}
	log.Debugf("Requested to refresh %d targets for job ID %d: %v", len(targets), jobID, targets)
	if len(targets) == 0 {
		return nil
	}

	_, err := d.handleLock(int64(jobID), targetIDList(targets), d.refreshTimeout, false)
	return err
}

// ResetAllLocks resets the database and clears all locks, regardless of who owns them.
// This is primarily for testing, and should not be used by used in prod, this
// is why it is not exposed by target.Locker
func (d *DBLocker) ResetAllLocks() error {
	log.Warning("DELETING ALL LOCKS")
	_, err := d.db.Exec("TRUNCATE TABLE locks;")
	return err
}

// Opt is a function type that sets parameters on the DBLocker object
type Opt func(dblocker *DBLocker)

// DriverName allows using a mysql-compatible driver (e.g. a wrapper around mysql
// or a syntax-compatible variant).
func DriverName(name string) Opt {
	return func(rdbms *DBLocker) {
		rdbms.driverName = name
	}
}

// New initializes and returns a new DBLocker target locker.
func New(dbURI string, lockTimeout, refreshTimeout time.Duration, opts ...Opt) (target.Locker, error) {
	res := &DBLocker{
		lockTimeout:    lockTimeout,
		refreshTimeout: refreshTimeout,
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
