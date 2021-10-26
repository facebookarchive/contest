// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package rdbms

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/event/frameworkevent"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/storage"
	log "github.com/sirupsen/logrus"

	"github.com/facebookincubator/contest/tools/migration/rdbms/migrationlib"

	// this blank import registers the mysql driver
	_ "github.com/go-sql-driver/mysql"
)

// txbeginner defines an interface for a backend which supports beginning a transaction
type txbeginner interface {
	Begin() (*sql.Tx, error)
}

// db defines an interface for a backend that supports Query and Exec Operations
type db interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// tx defines an interface for a backend that supports transaction like operations
type tx interface {
	Commit() error
	Rollback() error
}

// RDBMS implements a storage engine which stores ConTest information in a relational
// database via the database/sql package. With the current implementation, only MySQL
// is officially supported. Within MySQL, the current limitations are the following:
//
// It's not possible to use prepared statements. Not all MySQL connectors
// implementing database/sql support prepared statements, so the plugin cannot
// depend on them.
type RDBMS struct {
	dbURI, driverName string

	buffTestEvents      []testevent.Event
	buffFrameworkEvents []frameworkevent.Event

	testEventsLock      sync.Mutex
	frameworkEventsLock sync.Mutex
	closeCh             chan struct{}
	closeWG             *sync.WaitGroup

	// sql.Tx is not safe for concurrent use. This means that both Query, Exec operations
	// and rows scanning should be serialized. txLock is acquired and released by all
	// methods of the public interface exposed by RDBMS.
	txLock sync.Mutex

	db    db
	sqlDB *sql.DB

	// Events are buffered internally before being flushed to the database.
	// Buffer size and flush interval are defined per-buffer, as there is
	// a separate buffer for TestEvent and FrameworkEvent
	testEventsFlushSize          int
	testEventsFlushInterval      time.Duration
	frameworkEventsFlushSize     int
	frameworkEventsFlushInterval time.Duration
}

func (r *RDBMS) lockTx() {
	if _, ok := r.db.(tx); !ok {
		return
	}
	r.txLock.Lock()
}

func (r *RDBMS) unlockTx() {
	if _, ok := r.db.(tx); !ok {
		return
	}
	r.txLock.Unlock()
}

// BeginTx returns a storage.TransactionalStorage object backed by a transactional db object
func (r *RDBMS) BeginTx() (storage.TransactionalStorage, error) {

	txdb, ok := r.db.(txbeginner)
	if !ok {
		return nil, fmt.Errorf("backend does not support initiating a transaction")
	}

	tx, err := txdb.Begin()
	if err != nil {
		return nil, err
	}

	return &RDBMS{db: tx, sqlDB: r.sqlDB, closeCh: r.closeCh, closeWG: r.closeWG}, nil
}

// Commit persists the current transaction, if there is one active
func (r *RDBMS) Commit() error {
	tx, ok := r.db.(tx)
	if !ok {
		return fmt.Errorf("no active transaction")
	}
	return tx.Commit()
}

// Rollback rolls back the current transaction, if there is one active
func (r *RDBMS) Rollback() error {
	tx, ok := r.db.(tx)
	if !ok {
		return fmt.Errorf("no active transaction")
	}
	return tx.Rollback()
}

// Close flushes pending events and closes the database connection.
func (r *RDBMS) Close() error {
	close(r.closeCh)
	r.closeWG.Wait()
	r.sqlDB.Close()
	r.sqlDB = nil
	r.db = nil
	return nil
}

// Version returns the current version of the RDBMS schema
func (r *RDBMS) Version() (uint64, error) {
	return migrationlib.DBVersion(r.db)
}

// Reset wipes entire database contents. Used in tests.
func (r *RDBMS) Reset() error {
	for _, t := range []string{"jobs", "job_tags", "run_reports", "final_reports", "test_events", "framework_events"} {
		if _, err := r.db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", t)); err != nil {
			return err
		}
	}
	return nil
}

func (r *RDBMS) init() error {

	driverName := "mysql"
	if r.driverName != "" {
		driverName = r.driverName
	}
	sqlDB, err := sql.Open(driverName, r.dbURI)
	if err != nil {
		return fmt.Errorf("could not initialize database: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return fmt.Errorf("unable to contact database: %w", err)
	}
	r.db = sqlDB
	r.sqlDB = sqlDB

	if r.testEventsFlushInterval > 0 {
		r.closeWG.Add(1)
		go func() {
			done := false
			for !done {
				select {
				case <-time.After(r.testEventsFlushInterval):
					if err := r.flushTestEvents(); err != nil {
						log.Warningf("Failed to flush test events: %v", err)
					}
				case <-r.closeCh:
					// Flush one last time
					if err := r.flushTestEvents(); err != nil {
						log.Warningf("Failed to flush test events: %v", err)
					}
					done = true
				}
			}
			r.closeWG.Done()
		}()
	}

	if r.frameworkEventsFlushInterval > 0 {
		r.closeWG.Add(1)
		go func() {
			done := false
			for !done {
				select {
				case <-time.After(r.frameworkEventsFlushInterval):
					if err := r.flushFrameworkEvents(); err != nil {
						log.Warningf("Failed to flush test events: %v", err)
					}
				case <-r.closeCh:
					// Flush one last time
					if err := r.flushFrameworkEvents(); err != nil {
						log.Warningf("Failed to flush test events: %v", err)
					}
					done = true
				}
			}
			r.closeWG.Done()
		}()
	}
	return nil
}

// Opt is a function type that sets parameters on the RDBMS object
type Opt func(rdbms *RDBMS)

// TestEventsFlushSize defines maximum size of the test events buffer after which
// events are flushed to the database.
func TestEventsFlushSize(flushSize int) Opt {
	return func(rdbms *RDBMS) {
		rdbms.testEventsFlushSize = flushSize
	}
}

// TestEventsFlushInterval defines the interval at which buffered test events are
// stored into the database
func TestEventsFlushInterval(flushInterval time.Duration) Opt {
	return func(rdbms *RDBMS) {
		rdbms.testEventsFlushInterval = flushInterval
	}
}

// FrameworkEventsFlushSize defines maximum size of the framework events buffer
// after which events are flushed to the database.
func FrameworkEventsFlushSize(flushSize int) Opt {
	return func(rdbms *RDBMS) {
		rdbms.frameworkEventsFlushSize = flushSize
	}
}

// FrameworkEventsFlushInterval defines the interval at which buffered framework
// events are stored into the database
func FrameworkEventsFlushInterval(flushInterval time.Duration) Opt {
	return func(rdbms *RDBMS) {
		rdbms.frameworkEventsFlushInterval = flushInterval
	}
}

// DriverName allows using a mysql-compatible driver (e.g. a wrapper around mysql
// or a syntax-compatible variant).
func DriverName(name string) Opt {
	return func(rdbms *RDBMS) {
		rdbms.driverName = name
	}
}

// New creates a RDBMS events storage backend with default parameters
func New(dbURI string, opts ...Opt) (storage.Storage, error) {

	// Default flushInterval and buffer sizes are zero (i.e., by default the backend is not buffered)
	rdbms := RDBMS{
		dbURI:   dbURI,
		closeCh: make(chan struct{}),
		closeWG: &sync.WaitGroup{},
	}
	for _, Opt := range opts {
		Opt(&rdbms)
	}
	if err := rdbms.init(); err != nil {
		return nil, err
	}
	return &rdbms, nil
}
