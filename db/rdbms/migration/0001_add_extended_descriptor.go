// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migration

import (
    "database/sql"

    "github.com/facebookincubator/contest/tools/migration/rdbms/migrate"
    "github.com/go-sql-driver/mysql"
    "github.com/sirupsen/logrus"
)

type ExtendedDescriptorMigration struct {
    log *logrus.Entry
}

// Up implements the forward migration
func (m *ExtendedDescriptorMigration) Up(tx *sql.Tx) error {
    _, err := tx.Query("alter table jobs add column extended_descriptor text")
    if err != nil {
        if mysqlErr, ok := err.(*mysql.MySQLError); ok {
            if mysqlErr.Number == 1060 {
                m.log.Warningf("extended_descriptor column already exists, migration will have no effect")
                return nil
            }
        }
        return err
    }
    return nil
}

// Down implements the backward migration
func (m *ExtendedDescriptorMigration) Down(tx *sql.Tx) error {
    return nil
}

// NewExtendedDescriptorMigration is the factory for ExtendedDescriptorMigration
func NewExtendedDescriptorMigration(log *logrus.Entry) migrate.Migrate {
    return &ExtendedDescriptorMigration{log: log}
}

// register NewExtendedDescriptorMigration at initialization time
func init() {
    migrate.Register(NewExtendedDescriptorMigration)
}
