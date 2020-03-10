// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package mysql implements target.Locker using a MySQL table as the backend storage.
// The same table could be safely used by multiple instances of ConTest.
package mysql

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/facebookincubator/contest/pkg/lib/runtimetools"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"

	"github.com/facebookincubator/contest/plugins/targetlocker"
)

// enableDebug control if it's required to enable DEBUG logging level for
// this plugin.
var enableDebug = false

// TargetLocker is the MySQL-based target locker.
type TargetLocker struct {
	log     *logrus.Entry
	mySQL   *sql.DB
	querier mysqlLocksQuerier
}

// mysqlLocksQuerier implements MySQL-specific related logic.
//
// TargetLocker itself contains only logic related to generic SQL DBMS, and
// all MySQL-specific stuff is offloaded to mysqlLocksQuerier.
// It allows to easily implement support of another SQL DBMS in future
// (if required) and allows to read, write and check the logic separately
// ("how the locking is done" and "how is the querying to MySQL is done").
type mysqlLocksQuerier struct {
	log     *logrus.Entry
	lockTTL time.Duration
}

// backendQuerier is an abstraction over `*sql.Tx` and `*sql.DB`.
type backendQuerier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func (q mysqlLocksQuerier) tableName() string {
	return `locks`
}

// query is a wrapper around backendQuerier.Query to provide logs
func (q mysqlLocksQuerier) query(mysql backendQuerier, query string, args ...interface{}) (*sql.Rows, error) {
	startedAt := time.Now()
	rows, err := mysql.Query(query, args...)
	q.log.Debugf("[%v] query:<%v> args:%v -> rows_is_notNil:%v, err:%v", time.Since(startedAt), query, args, rows != nil, err)
	return rows, err
}

// exec is a wrapper around backendQuerier.Exec to provide logs.
func (q mysqlLocksQuerier) exec(mysql backendQuerier, query string, args ...interface{}) (sql.Result, error) {
	startedAt := time.Now()
	result, err := mysql.Exec(query, args...)
	var rowsAffectedAmount int64
	var rowsAffectedError error
	if result != nil {
		rowsAffectedAmount, rowsAffectedError = result.RowsAffected()
	}
	q.log.Debugf("[%v] exec:<%v> args:%v -> result:%v<rowsAffected:{amount:%v err:%v}>, err:%v",
		time.Since(startedAt), query, args, result, rowsAffectedAmount, rowsAffectedError, err)
	return result, err
}

// checkAccess performs a test query to check if we have access to the data.
func (q mysqlLocksQuerier) checkAccess(mysql backendQuerier) error {
	rows, err := q.query(mysql, "SELECT * FROM `"+q.tableName()+"` LIMIT 1") // #nosec G202
	if err == nil {
		_ = rows.Close()
	}
	return err
}

// do performs a Lock, Unlock or RefreshLock (depending on value of `action`).
func (q mysqlLocksQuerier) do(
	action targetlocker.Action,
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) error {
	var fn func(backendQuerier, types.JobID, string) error
	switch action {
	case targetlocker.ActionLock:
		fn = q.actionLock
	case targetlocker.ActionUnlock:
		fn = q.actionUnlock
	case targetlocker.ActionRefreshLock:
		fn = q.actionRefreshLock
	default:
		return targetlocker.ErrInvalidAction{Action: action}
	}
	return fn(mysql, jobID, targetID)
}

// actionClearExpiredLock removes expired locks for a selected target
func (q mysqlLocksQuerier) actionClearExpiredLock(
	mysql backendQuerier,
	targetID string,
) error {
	_, err := q.exec(mysql,
		"DELETE FROM `"+q.tableName()+"` WHERE target_id=? AND NOW() >= expires_at",
		targetID) // #nosec G202
	return err
}

type mysqlErrorType int

const (
	mysqlErrorTypeUndefined = mysqlErrorType(iota)
	mysqlErrorTypeNoError
	mysqlErrorTypeUnknown
	mysqlErrorTypeOther
	mysqlErrorTypeNoRowsAffected
	mysqlErrorTypeDuplicateKey
)

func (errType mysqlErrorType) String() string {
	switch errType {
	case mysqlErrorTypeUndefined:
		return "undefined"
	case mysqlErrorTypeNoError:
		return "no_error"
	case mysqlErrorTypeUnknown:
		return "unknown"
	case mysqlErrorTypeOther:
		return "other"
	case mysqlErrorTypeNoRowsAffected:
		return "no_rows"
	case mysqlErrorTypeDuplicateKey:
		return "duplicate_key"
	}
	return "invalid"
}

// errorType detects the kind of a MySQL error.
//
// So far it used only to detect if the error is a "Duplicate Key" error.
func (q mysqlLocksQuerier) errorType(err error) mysqlErrorType {
	_ = mysqlErrorTypeUndefined // just to bypass linter error: `mysqlErrorTypeUndefined` is unused (deadcode)

	if err == nil {
		return mysqlErrorTypeNoError
	}

	if xerrors.Is(err, sql.ErrNoRows) {
		return mysqlErrorTypeNoRowsAffected
	}

	var mysqlError *mysql.MySQLError
	if !xerrors.As(err, &mysqlError) {
		return mysqlErrorTypeUnknown
	}
	switch mysqlError.Number {
	case 1062:
		return mysqlErrorTypeDuplicateKey
	}
	return mysqlErrorTypeOther
}

// actionLock inserts a lock for a target (and removes an expired lock
// if there's any).
func (q mysqlLocksQuerier) actionLock(
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) error {
	// Remove expired an lock (if exists)

	_ = q.actionClearExpiredLock(mysql, targetID)

	// Add the lock

	var errs []error

	result, execErr := q.exec(mysql,
		"INSERT INTO `"+q.tableName()+"` "+
			"SET target_id=?, job_id=?, created_at = NOW(), expires_at = NOW() + INTERVAL ? SECOND",
		targetID, jobID, int64(q.lockTTL.Seconds())) // #nosec G202

	// Check errors:

	if execErr != nil {
		errs = append(errs, execErr)
	}

	if result != nil {
		_, rowsAffectedErr := result.RowsAffected()
		if rowsAffectedErr != nil {
			errs = append(errs, rowsAffectedErr)
		}
	}

	for _, errItem := range errs {
		switch q.errorType(errItem) {
		case mysqlErrorTypeNoError, mysqlErrorTypeNoRowsAffected:
			continue
		case mysqlErrorTypeUndefined, mysqlErrorTypeUnknown:
			return fmt.Errorf("actionLock(): unexpected error: %w", errItem)
		case mysqlErrorTypeDuplicateKey:
			return targetlocker.ErrAlreadyLocked{
				TargetID:        targetID,
				UnderlyingError: errItem,
			}
		}
	}

	return nil
}

// actionUnlock removes a lock for a target (if it is owned by job `JobID`).
func (q mysqlLocksQuerier) actionUnlock(
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) error {
	_, err := q.exec(mysql,
		"DELETE FROM `"+q.tableName()+"` "+
			"WHERE target_id=? AND job_id=?", targetID, jobID) // #nosec G202
	return err
}

// actionRefreshLock updates `expires_at` of a lock for a target (if it owned by job `JobID`).
func (q mysqlLocksQuerier) actionRefreshLock(
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) error {
	_, err := q.exec(mysql,
		"UPDATE `"+q.tableName()+"` "+
			`SET expires_at = NOW() + INTERVAL ? SECOND WHERE target_id=? AND job_id=?`,
		int64(q.lockTTL.Seconds()), targetID, jobID) // #nosec G202
	return err
}

// placeholder returns a strings of placeholders to fit the argument `arg`
// for further use by `backendQuerier.Query`/`backendQuerier.Exec`.
//
// For example slice ["a", "b", "c"] will get a placeholder "(?, ?, ?)".
//
// See also `arguments` below.
func (q mysqlLocksQuerier) placeholder(arg interface{}) string {
	switch arg := arg.(type) {
	case []string:
		if len(arg) == 0 {
			return `()`
		}
		result := strings.Repeat(`?,`, len(arg))
		return "(" + result[:len(result)-1] + ")"
	default:
		panic(fmt.Sprintf("unknown argument type: %T", arg))
	}
}

// arguments returns argument `arg` as arguments for
// `backendQuerier.Query`/`backendQuerier.Exec`
//
// For example slice []string{"a", "b", "c"} will be converted to []interface{}{"a", "b", "c"}.
//
// See also `placeholder` above.
func (q mysqlLocksQuerier) arguments(arg interface{}) (result []interface{}) {
	switch arg := arg.(type) {
	case []string:
		for _, item := range arg {
			result = append(result, item)
		}
	default:
		panic(fmt.Sprintf("unknown argument type: %T", arg))
	}
	return
}

// getLocks returns `*sql.Row` which contains target_id-s (string) of
// all targets from list `targetIDs` which are still locked by job `jobID`.
//
// See an usage example in `CheckLocks`.
func (q mysqlLocksQuerier) getLocks(
	mysql backendQuerier,
	jobID types.JobID,
	targetIDs []string,
) (*sql.Rows, error) {
	arguments := []interface{}{jobID}
	arguments = append(arguments, q.arguments(targetIDs)...)
	return q.query(mysql,
		"SELECT `target_id` FROM `"+q.tableName()+"` "+
			"WHERE NOW() < expires_at AND job_id=? AND target_id IN "+q.placeholder(targetIDs),
		arguments...) // #nosec G202
}

// reportIfError reports the error `err` only if `err != nil`.
func (tl *TargetLocker) reportIfError(err error) {
	if err == nil {
		return
	}

	// We extract the frame because we want to report about the line of the code
	// which called this method (`reportIfError`). We don't want for errors
	// reported from different places of the code be indistinctible
	// from each other.
	frame := runtimetools.Frame(1)
	tl.log.Errorf("%v:%v:%v: %v", frame.File, frame.Line, frame.Function, err)
}

// tx is a wrapper to perform a transaction.
//
// if `fn` returns an error then the transaction will be rolled-back,
// if `fn` returns nil then the transaction will be committed.
func (tl *TargetLocker) tx(description string, fn func(tx *sql.Tx) error) (err error) {
	tx, err := tl.mySQL.Begin()
	if err != nil {
		return fmt.Errorf("unable to '%s': unable to start transaction: %w", description, err)
	}

	tl.log.Debugf("Start transaction -> %p", tx)

	defer func() {
		if err != nil {
			tl.log.Debugf("Rollback %p due to error %v", tx, err)
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				tl.log.Errorf("unable to rollback '%s' transaction %p: %v", description, tx, rollbackErr)
			}
			return
		}

		tl.log.Debugf("Commit <- %p", tx)
		err = tx.Commit()
		if err != nil {
			err = fmt.Errorf("unable to '%s': unable to commit transaction %p: %w", description, tx, err)
		}
	}()

	return fn(tx)
}

// performActionOnTargets performs action `action` for all targets of `targets`
// as a single transaction. The transaction will be rolled-back if at least
// one error has occurred.
func (tl *TargetLocker) performActionOnTargets(
	action targetlocker.Action,
	jobID types.JobID,
	targets target.Targets,
) error {
	tl.log.Debugf("'%s' on targets: %v", action, targets)
	return tl.tx(action.String(), func(tx *sql.Tx) (err error) {
		for _, targetItem := range targets {
			err := tl.querier.do(action, tx, jobID, targetItem.ID)
			if err != nil {
				return targetlocker.ErrUnableToPerformAction{
					Action: action, JobID: jobID, Target: targetItem,
					Err: fmt.Errorf("got an error from Exec(): %w", err),
				}
			}
		}
		return nil
	})
}

// Lock locks the specified targets. The timeout must be handled by the
// plugin, and configured at construction time by the plugin's
// Factory.
//
// The job ID is the owner of the lock.
// The underlying implementation is responsible for using the job ID as lock
// owner, and for unlocking the already-locked ones in case of errors.
func (tl *TargetLocker) Lock(jobID types.JobID, targets []*target.Target) (err error) {
	return tl.performActionOnTargets(targetlocker.ActionLock, jobID, targets)
}

// Unlock unlocks the specified targets, with the specified job ID as owner.
// The underlying implementation is responsible of rejecting the operation if
// the lock owner is not matching the job ID.
func (tl *TargetLocker) Unlock(jobID types.JobID, targets []*target.Target) error {
	return tl.performActionOnTargets(targetlocker.ActionUnlock, jobID, targets)
}

// CheckLocks returns whether all the targets are locked by the given job ID,
// an array of locked targets, and an array of not-locked targets.
func (tl *TargetLocker) CheckLocks(
	jobID types.JobID,
	targets []*target.Target,
) (locked []*target.Target, notLocked []*target.Target, err error) {
	tl.log.Debugf("Check locks on targets: %v", targets)
	defer func() { tl.log.Debugf("Check locks -> %v %v %v", locked, notLocked, err) }()
	allTargets := target.Targets(targets)
	allTargets.Sort()

	// Get information about active locks (owned by JobID and relevant to selected targets)

	validLocksRows, err := tl.querier.getLocks(tl.mySQL, jobID, allTargets.IDs())
	if err != nil {
		return nil, nil, targetlocker.ErrUnableToGetLocksInfo{Err: fmt.Errorf("unable to SELECT: %w", err)}
	}
	defer func() { tl.reportIfError(validLocksRows.Close()) }()

	// Fill "locked"

	for validLocksRows.Next() {
		var targetID string
		err := validLocksRows.Scan(&targetID)
		if err != nil {
			return nil, nil, targetlocker.ErrUnableToGetLocksInfo{Err: fmt.Errorf("unable to parse: %w", err)}
		}
		validTarget := allTargets.Find(targetID)
		if validTarget == nil {
			return nil, nil, fmt.Errorf("internal error, target %s not found in %v", targetID, allTargets)
		}
		locked = append(locked, validTarget)
	}

	// Fill "notLocked"

	if len(locked) == len(allTargets) {
		return
	}
	notLocked = make([]*target.Target, 0, len(allTargets)-len(locked))
	// "allTargets" and "locked" are sorted, so we may use this simple loop to exclude intersection:
	for allIdx, lockedIdx := 0, 0; allIdx < len(allTargets); {
		if lockedIdx < len(locked) && allTargets[allIdx].ID == locked[lockedIdx].ID {
			allIdx++
			lockedIdx++
			continue
		}
		notLocked = append(notLocked, allTargets[allIdx])
		allIdx++
	}

	return
}

// RefreshLocks extends the lock. The amount of time the lock is extended
// should be handled by the plugin, and configured at initialization time.
// The request is rejected if the job ID does not match the one of the lock
// owner.
func (tl *TargetLocker) RefreshLocks(jobID types.JobID, targets []*target.Target) error {
	return tl.performActionOnTargets(targetlocker.ActionRefreshLock, jobID, targets)
}

// checkAccess returns an error if detected a problem with access to the data.
func (tl *TargetLocker) checkAccess() error {
	err := tl.querier.checkAccess(tl.mySQL)
	switch tl.querier.errorType(err) {
	case mysqlErrorTypeNoError, mysqlErrorTypeNoRowsAffected:
		return nil
	default:
		return fmt.Errorf(`unable to select from table '%s': %w`, tl.querier.tableName(), err)
	}
}

// Factory is the implementation of target.LockerFactory based
// on TargetLocker of this package.
type Factory struct{}

// New initializes and returns a new target locker based DBMS storage.
//
// "dsn" is the "Data Source Name", see: https://github.com/go-sql-driver/mysql/
func (f *Factory) New(timeout time.Duration, dsn string) (target.Locker, error) {
	if timeout < time.Second {
		return nil, targetlocker.ErrNonPositiveTimeout{Timeout: timeout}
	}

	mysqlCfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, targetlocker.ErrInvalidDSN{Err: err}
	}
	mysqlCfg.MultiStatements = true
	dsn = mysqlCfg.FormatDSN()

	mysqlClient, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize backend client to '%s': %w", dsn, err)
	}

	locker, err := f.new(timeout, mysqlClient)
	if err == nil {
		locker.log.Debugf("initialized mysqlClient client with DSN '%s'", dsn)
	}
	return locker, err
}

// new is the part of New which is useful for unit-tests.
func (f *Factory) new(timeout time.Duration, mysqlClient *sql.DB) (*TargetLocker, error) {
	loggerID := "targetlocker/" + strings.ToLower(f.UniqueImplementationName())
	tl := &TargetLocker{
		mySQL: mysqlClient,
		log:   logging.GetLogger(loggerID),
		querier: mysqlLocksQuerier{
			log:     logging.GetLogger(loggerID + ":query"),
			lockTTL: timeout,
		},
	}

	if enableDebug {
		if tl.log.Logger.Level < logrus.DebugLevel {
			tl.log.Logger.SetLevel(logrus.DebugLevel)
		}
		tl.log.Level = logrus.DebugLevel
		tl.querier.log.Level = logrus.DebugLevel
	}

	if err := tl.checkAccess(); err != nil {
		return nil, fmt.Errorf("access problem: %w", err)
	}
	return tl, nil
}

// UniqueImplementationName returns the unique name of the implementation
func (f *Factory) UniqueImplementationName() string {
	return "MySQL"
}
