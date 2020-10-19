// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	// Import migration packages so that golang migrations can register themselves
	_ "github.com/facebookincubator/contest/db/rdbms/migration"

	"github.com/facebookincubator/contest/tools/migration/rdbms/migrate"

	_ "github.com/go-sql-driver/mysql"

	"github.com/pressly/goose"
	"github.com/sirupsen/logrus"
)

var (
	flags        = flag.NewFlagSet("migrate", flag.ExitOnError)
	flagDBDriver = flags.String("dbDriver", "mysql", "DB Driver")
	flagDBURI    = flags.String("dbURI", "contest:contest@tcp(localhost:3306)/contest?parseTime=true", "Database URI")
	flagDir      = flags.String("dir", "", "Directory containing migration scripts")
	flagDebug    = flags.Bool("debug", false, "Enabled debug logging")
)

var usageHeader = `Usage: migrate [OPTIONS] COMMAND`
var commandsUsage = `
Commands:
    up                   Migrate the DB to the most recent version available
    up-by-one            Migrate the DB up by 1
    up-to VERSION        Migrate the DB to a specific VERSION
    down                 Roll back the version by 1
    down-to VERSION      Roll back to a specific VERSION
    redo                 Re-run the latest migration
    reset                Roll back all migrations
    status               Dump the migration status for the current DB
    version              Print the current version of the database
    create NAME [sql|go] Creates new migration file with the current timestamp
    fix                  Apply sequential ordering to migrations
`

func usage() {
	buf := new(bytes.Buffer)
	flags.SetOutput(buf)
	flags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "%s", usageHeader)
	fmt.Fprintf(os.Stderr, "%s", "\n")
	fmt.Fprintf(os.Stderr, "%s", buf.String())
	fmt.Fprintf(os.Stderr, "%s", commandsUsage)
}

func main() {

	var log = logrus.New()

	if len(os.Args) < 2 {
		flags.Usage()
		return
	}

	flags.Usage = usage
	err := flags.Parse(os.Args[1:])
	if err != nil {
		flags.Usage()
		panic(err)
	}

	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.InfoLevel)
	if *flagDebug {
		log.SetLevel(logrus.DebugLevel)
	}

	if *flagDir == "" {
		flags.Usage()
		log.Fatalf("migration directory was not specified")
	}

	for _, m := range migrate.Migrations {
		logger := log.WithField("migration", filepath.Base(m.Name))
		migration := m.Factory(logger)
		goose.AddNamedMigration(m.Name, migration.Up, migration.Down)
	}

	command := os.Args[len(os.Args)-1]
	db, err := goose.OpenDBWithDriver(*flagDBDriver, *flagDBURI)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("db not reachable: %v", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("failed to close DB: %v", err)
		}
	}()

	if err := goose.Run(command, db, *flagDir, flags.Args()...); err != nil {
		log.Fatalf("could not run command %v for migration: %v", command, err)
	}
}
