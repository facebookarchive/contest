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

var enableDebug = false

// TargetLocker is the no-op target locker. It does nothing.
type TargetLocker struct {
	log     *logrus.Entry
	mySQL   *sql.DB
	querier mysqlLocksQuerier
}

type mysqlLocksQuerier struct {
	log     *logrus.Entry
	lockTTL time.Duration
}

type backendQuerier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func (q mysqlLocksQuerier) tableName() string {
	return `locks`
}

func (q mysqlLocksQuerier) query(mysql backendQuerier, query string, args ...interface{}) (*sql.Rows, error) {
	startedAt := time.Now()
	rows, err := mysql.Query(query, args...)
	q.log.Debugf("[%v] query:<%v> args:%v -> rows_is_notNil:%v, err:%v", time.Since(startedAt), query, args, rows != nil, err)
	return rows, err
}

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

func (q mysqlLocksQuerier) checkAccess(mysql backendQuerier) (*sql.Rows, error) {
	return q.query(mysql, "SELECT * FROM `"+q.tableName()+"` LIMIT 1") // #nosec G202
}

func (q mysqlLocksQuerier) do(
	action targetlocker.Action,
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) (sql.Result, error) {
	var fn func(backendQuerier, types.JobID, string) (sql.Result, error)
	switch action {
	case targetlocker.ActionLock:
		fn = q.actionLock
	case targetlocker.ActionUnlock:
		fn = q.actionUnlock
	case targetlocker.ActionRefreshLock:
		fn = q.actionRefreshLock
	default:
		return nil, targetlocker.ErrInvalidAction{Action: action}
	}
	return fn(mysql, jobID, targetID)
}

func (q mysqlLocksQuerier) actionClearExpiredLock(
	mysql backendQuerier,
	targetID string,
) (sql.Result, error) {
	return q.exec(mysql,
		"DELETE FROM `"+q.tableName()+"` WHERE target_id=? AND NOW() >= expires_at",
		targetID) // #nosec G202
}

type mysqlErrorType int

const (
	mysqlErrorTypeUndefined = mysqlErrorType(iota)
	mysqlErrorTypeUnknown
	mysqlErrorTypeOther
	mysqlErrorTypeDuplicateKey
)

func (q mysqlLocksQuerier) errorType(err error) mysqlErrorType {
	_ = mysqlErrorTypeUndefined // just to bypass linter error: `mysqlErrorTypeUndefined` is unused (deadcode)

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

func (q mysqlLocksQuerier) actionLock(
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) (result sql.Result, err error) {
	// Remove expired an lock (if exists)

	_, _ = q.actionClearExpiredLock(mysql, targetID)

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
		if q.errorType(errItem) == mysqlErrorTypeDuplicateKey {
			err = targetlocker.ErrAlreadyLocked{
				TargetID:        targetID,
				UnderlyingError: errItem,
			}
		}
	}

	return
}

func (q mysqlLocksQuerier) actionUnlock(
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) (sql.Result, error) {
	return q.exec(mysql,
		"DELETE FROM `"+q.tableName()+"` "+
			"WHERE target_id=? AND job_id=?", targetID, jobID) // #nosec G202
}

func (q mysqlLocksQuerier) actionRefreshLock(
	mysql backendQuerier,
	jobID types.JobID,
	targetID string,
) (sql.Result, error) {
	return q.exec(mysql,
		"UPDATE `"+q.tableName()+"` "+
			`SET expires_at = NOW() + INTERVAL ? SECOND WHERE target_id=? AND job_id=?`,
		int64(q.lockTTL.Seconds()), targetID, jobID) // #nosec G202
}

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

func (q mysqlLocksQuerier) getLocks(
	mysql backendQuerier,
	jobID types.JobID,
	targetIDs []string,
) (*sql.Rows, error) {
	arguments := []interface{}{jobID}
	arguments = append(arguments, q.arguments(targetIDs)...)
	return q.query(mysql,
		"SELECT `target_id` FROM `"+q.tableName()+"` "+
			"WHERE job_id=? AND target_id IN "+q.placeholder(targetIDs),
		arguments...) // #nosec G202
}

func (tl *TargetLocker) reportIfError(err error) {
	if err == nil {
		return
	}
	frame := runtimetools.Frame(1)
	tl.log.Errorf("%v:%v:%v: %v", frame.File, frame.Line, frame.Function, err)
}

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

func (tl *TargetLocker) performActionOnTargets(
	action targetlocker.Action,
	jobID types.JobID,
	targets []*target.Target,
) error {
	tl.log.Debugf("'%s' on targets: %v", action, targets)
	return tl.tx(action.String(), func(tx *sql.Tx) (err error) {
		for _, targetItem := range targets {
			result, err := tl.querier.do(action, tx, jobID, targetItem.ID)
			if err != nil {
				return targetlocker.ErrUnableToPerformAction{
					Action: action, JobID: jobID, Target: targetItem,
					Err: fmt.Errorf("got an error from Exec(): %w", err),
				}
			}

			rowsAffectedAmount, rowsAffectedErr := result.RowsAffected()
			if rowsAffectedErr != nil {
				return targetlocker.ErrUnableToPerformAction{
					Action: action, JobID: jobID, Target: targetItem,
					Err: fmt.Errorf("got an error from RowsAffected(): %w", rowsAffectedErr)}
			}
			if rowsAffectedAmount == 0 {
				return targetlocker.ErrUnableToPerformAction{
					Action: action, JobID: jobID, Target: targetItem,
					Err: fmt.Errorf("this should never happen this way, but: %w", sql.ErrNoRows),
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

func (tl *TargetLocker) checkAccess() error {
	rows, err := tl.querier.checkAccess(tl.mySQL)
	switch {
	case err == nil:
		tl.reportIfError(rows.Close())
	case err != sql.ErrNoRows:
		return fmt.Errorf(`unable to select from table '%s': %w`, tl.querier.tableName(), err)
	}
	return nil
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

func (f *Factory) new(timeout time.Duration, mysqlClient *sql.DB) (*TargetLocker, error) {
	loggerID := "teststeps/" + strings.ToLower(f.UniqueImplementationName())
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
