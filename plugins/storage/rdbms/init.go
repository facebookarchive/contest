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
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/storage"

	"github.com/facebookincubator/contest/tools/migration/rdbms/migrationlib"

	// this blank import registers the mysql driver
	_ "github.com/go-sql-driver/mysql"
)

var log = logging.GetLogger("plugin/events/rdbms")

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

	testEventsLock      *sync.Mutex
	frameworkEventsLock *sync.Mutex

	// sql.Tx is not safe for concurrent use. This means that both Query, Exec operations
	// and rows scanning should be serialized. txLock is acquired and released by all
	// methods of the public interface exposed by RDBMS.
	txLock sync.Mutex

	db db

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
	txRDBMS := RDBMS{testEventsLock: &sync.Mutex{}, frameworkEventsLock: &sync.Mutex{}, txLock: sync.Mutex{}, db: tx}
	return &txRDBMS, nil
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

// Version returns the current version of the RDBMS schema
func (r *RDBMS) Version() (int64, error) {
	if sqlDB, ok := r.db.(*sql.DB); ok {
		return migrationlib.DBVersion(sqlDB)
	}
	return 0, fmt.Errorf("db object (%T) is not a sql.DB", r.db)
}

func (r *RDBMS) init() error {

	driverName := "mysql"
	if r.driverName != "" {
		driverName = r.driverName
	}
	sqlDb, err := sql.Open(driverName, r.dbURI)
	if err != nil {
		return fmt.Errorf("could not initialize database: %v", err)
	}
	r.db = sqlDb

	if r.testEventsFlushInterval > 0 {
		go func() {
			for {
				time.Sleep(r.testEventsFlushInterval)

				r.testEventsLock.Lock()
				if err := r.flushTestEvents(); err != nil {
					log.Warningf("Failed to flush test events: %v", err)
				}
				r.testEventsLock.Unlock()
			}
		}()
	}

	if r.frameworkEventsFlushInterval > 0 {
		go func() {
			for {
				time.Sleep(r.frameworkEventsFlushInterval)

				r.frameworkEventsLock.Lock()
				if err := r.flushFrameworkEvents(); err != nil {
					log.Warningf("Failed to flush test events: %v", err)
				}
				r.frameworkEventsLock.Unlock()
			}
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
	rdbms := RDBMS{testEventsLock: &sync.Mutex{}, frameworkEventsLock: &sync.Mutex{}, dbURI: dbURI}
	for _, Opt := range opts {
		Opt(&rdbms)
	}
	if err := rdbms.init(); err != nil {
		return nil, err
	}
	return &rdbms, nil
}
