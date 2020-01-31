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

	// this blank import registers the mysql driver
	_ "github.com/go-sql-driver/mysql"
)

var log = logging.GetLogger("plugin/events/rdbms")

const defaultFlushSize int = 64
const defaultFlushInterval time.Duration = 5 * time.Second

// RDBMS implements a storage engine which stores ConTest information in a relational
// database via the database/sql package. With the current implementation, only MySQL
// is officially supported. Within MySQL, the current limitations are the following:
//
// It's not possible to use prepared statements. Not all MySQL connectors
// implementing database/sql support prepared statements, so the plugin cannot
// depend on them.
type RDBMS struct {
	driverName          string
	buffTestEvents      []testevent.Event
	buffFrameworkEvents []frameworkevent.Event

	initOnce *sync.Once

	testEventsLock      *sync.Mutex
	frameworkEventsLock *sync.Mutex

	db *sql.DB

	dbURI string

	// Events are buffered internally before being flushed to the database.
	// Buffer size and flush interval are defined per-buffer, as there is
	// a separate buffer for TestEvent and FrameworkEvent
	testEventsFlushSize          int
	testEventsFlushInterval      time.Duration
	frameworkEventsFlushSize     int
	frameworkEventsFlushInterval time.Duration
}

// Reset restores a clean state in the database. It's meant to be used after
// integration tests. As it's a potentially dangerous operation, it's not part
// of the  EventsStorage interface.
func (r *RDBMS) Reset() error {

	if err := r.init(); err != nil {
		return fmt.Errorf("could not initialize database: %v", err)
	}
	_, err := r.db.Exec("truncate test_events")
	if err != nil {
		return fmt.Errorf("could not truncate table events: %v", err)
	}
	_, err = r.db.Exec("truncate framework_events")
	if err != nil {
		return fmt.Errorf("could not truncate table framework_events: %v", err)
	}
	_, err = r.db.Exec("truncate jobs")
	if err != nil {
		return fmt.Errorf("could not truncate table jobs: %v", err)
	}
	_, err = r.db.Exec("truncate reports")
	if err != nil {
		return fmt.Errorf("could not truncate table reports: %v", err)
	}
	return nil
}

func (r *RDBMS) init() error {
	initFunc := func() error {
		driverName := "mysql"
		if r.driverName != "" {
			driverName = r.driverName
		}
		db, err := sql.Open(driverName, r.dbURI)
		if err != nil {
			return fmt.Errorf("could not initialize database for events: %v", err)
		}
		r.db = db
		// Background goroutines for flushing pending events. The lifetime of the
		// goroutines correspond to the lifetime of the framework.
		go func() {
			for {
				time.Sleep(r.testEventsFlushInterval)
				r.FlushTestEvents()
			}
		}()

		go func() {
			for {
				time.Sleep(r.frameworkEventsFlushInterval)
				r.FlushFrameworkEvents()
			}
		}()
		return err
	}

	var initErr error
	r.initOnce.Do(func() {
		initErr = initFunc()
	})
	return initErr
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
func New(dbURI string, opts ...Opt) storage.Storage {
	backend := RDBMS{
		dbURI:                        dbURI,
		testEventsLock:               &sync.Mutex{},
		frameworkEventsLock:          &sync.Mutex{},
		initOnce:                     &sync.Once{},
		testEventsFlushSize:          defaultFlushSize,
		testEventsFlushInterval:      defaultFlushInterval,
		frameworkEventsFlushSize:     defaultFlushSize,
		frameworkEventsFlushInterval: defaultFlushInterval,
	}
	for _, Opt := range opts {
		Opt(&backend)
	}
	return &backend
}
