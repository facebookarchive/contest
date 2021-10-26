// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migrate

import (
	"database/sql"
	"runtime"

	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Migrate is the interface that every migration task must implement to support
type Migrate interface {
	Up(tx *sql.Tx) error
	Down(tx *sql.Tx) error
	UpNoTx(db *sql.DB) error
	DownNoTx(db *sql.DB) error
}

// Factory defines a factory type of an object implementing Migration interface
type Factory func(ctx xcontext.Context) Migrate

// Migration represents a migration task registered in the migration tool
type Migration struct {
	Factory Factory
	Name    string
}

// Migrations represents a sets of migrations
var Migrations []Migration

// Register registers a new factory for a migration
func Register(Factory Factory) {
	_, filename, _, _ := runtime.Caller(1)
	newMigration := Migration{Factory: Factory, Name: filename}
	Migrations = append(Migrations, newMigration)
}
