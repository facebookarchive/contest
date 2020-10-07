// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	dbmigration "github.com/facebookincubator/contest/db/rdbms/migration"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/tools/migration"

	"github.com/sirupsen/logrus"

	"github.com/gosuri/uiprogress"
)

const migrationPath = "db/rdbms/migration"

var (
	flagDBURI = flag.String("dbURI", config.DefaultDBURI, "Database URI")
	flagSRC   = flag.Uint("from", 0, "Initial version of the schema")
	flagDST   = flag.Uint("to", 0, "Destination version of the schema")
)

func main() {
	flag.Parse()

	log := logging.GetLogger("contest")
	log.Level = logrus.DebugLevel

	var (
		vSrc uint
		vDst uint
	)
	if *flagSRC == 0 {
		log.Fatalf("initial version of the schema cannot be 0")
	}

	if *flagDST == 0 {
		log.Fatalf("destination version of the schema cannot be 0")
	}

	if *flagSRC > *flagDST {
		log.Fatalf("initial version of the schema m4ust be lower than final version")
	}

	dbURI := *flagDBURI

	vSrc = *flagSRC
	vDst = *flagDST

	tasks := migration.NewTasks()

	availableTasks := []migration.Task{dbmigration.TestNameMigration{}, dbmigration.EventNameMigration{}}
	for _, task := range availableTasks {
		err := tasks.Register(task)
		if err != nil {
			log.Fatalf("could not register `%s` migration task: %v", task.Desc(), err)
		}
	}

	log.Infof("beginning schema migration from %d to %d", vSrc, vDst)

	db, err := sql.Open("mysql", dbURI)
	if err != nil {
		log.Fatalf("could not open mysql URI: %s", dbURI)
	}
	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		log.Fatalf("could not open mysql instance: %+v", err)
	}

	log.Infof("beginning migration v%d -> v%d", vSrc, vDst)
	m, err := migrate.NewWithInstance("tasks", tasks, "mysql", driver)
	if err != nil {
		log.Fatalf("could not create instace for migration v%d -> v%d: %+v", vSrc, vDst, err)
	}

	currentVersion, dirty, err := m.Version()
	if err != nil {
		// TODO consider the fact that there might be no migration in progress
		log.Warningf("could not get the currrent active migration version: %+v", err)
	}

	if dirty {
		log.Fatalf("current version (v%d) has not completed migration correctly, this needs to fixed manually", currentVersion)
	}

	if currentVersion >= vDst {
		log.Fatalf("current version (v%d) does not allow to migrate to v%d", currentVersion, vDst)
	}

	uiprogress.Start()

	for {
		if currentVersion >= vDst {
			break
		}

		err = m.Steps(1)
		if err != nil {
			log.Fatalf("could not run migration v%d: %+v", currentVersion, err)
		}

		currentVersion++
		t := tasks.GetTask(currentVersion)
		if t == nil {
			log.Warningf("task to v%d migration is not available", currentVersion)
			continue
		}

		terminateCh := make(chan struct{})
		progressCh := make(chan *migration.Progress)
		errCh := make(chan error)
		go func() {
			errCh <- t.MigrateData(db, terminateCh, progressCh)
		}()

		progress := uiprogress.New()
		bar := progress.AddBar(100)
		bar.AppendCompleted()
		bar.PrependFunc(func(b *uiprogress.Bar) string {
			return fmt.Sprintf("Task: %s", t.Desc())
		})

		progress.Start()

		var errTask error
		for {
			select {

			case p := <-progressCh:
				if p.Total != 0 {
					bar.Set(int((float64(p.Completed) / float64(p.Total)) * 100))
				}
			case errTask = <-errCh:
				errCh = nil
			}

			if errTask != nil || errCh == nil {
				bar.Set(100)
				time.Sleep(500 * time.Millisecond)
				break
			}
		}
		if errTask != nil {
			log.Fatalf("task %d failed to run: %+v", currentVersion, errTask)
		}
		progress.Stop()

		log.Infof("migration v%d completed!", currentVersion)
	}

	log.Infof("all migrations have completed successfully!")
}
