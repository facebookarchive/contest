// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration_storage

package test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/facebookincubator/contest/db/rdbms/migration"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/tests/integ/common"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestDescriptorMigrationSuite implements tests for `0002_migrate_descriptor_to_extended_descriptor` migration
type TestDescriptorMigrationSuite struct {
	suite.Suite

	// current transaction to be used for database operations
	tx *sql.Tx
}

// SetupTest initializes the transaction storage engine and adds to the underlying db test entries that are migrated during the test
func (suite *TestDescriptorMigrationSuite) SetupTest() {
	// Setup raw connection to the db. We cannoit use storage plugin directly
	// because we need to populate data into the db in a format
	db, err := sql.Open("mysql", common.GetDatabaseURI())
	if err != nil {
		panic(fmt.Sprintf("could not initialize TestDescriptorMigrationSuite: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		panic(fmt.Sprintf("could not reach database: %v", err))
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		panic(fmt.Sprintf("could not initiate transaction: %v", err))
	}
	suite.tx = tx

	// Populate db with data that will be used to test the migration

	insertStatement := "insert into jobs (name, descriptor, teststeps, requestor, server_id, request_time) values (?, ?, ?, ?, ?, ?)"

	jobName := "TestJobName"
	requestor := "TestRequestor"
	serverID := "TestServerId"
	requestTime := time.Now()
	_, err = tx.Exec(insertStatement, jobName, jobDescriptor, testSerialized, requestor, serverID, requestTime)
	if err != nil {
		panic(fmt.Sprintf("could not initialize database with test data: %v", err))
	}

}

func (suite *TestDescriptorMigrationSuite) TearDownTest() {
	if err := suite.tx.Rollback(); err != nil {
		panic(fmt.Sprintf("could not rollback transaction: %v", err))
	}
}

// TestFetchJobs tests that jobs are fetched correctly from the db
func (suite *TestDescriptorMigrationSuite) TestUpMigratesToExtendedDescriptor() {

	log := logrus.New()
	fields := logrus.Fields{"test": "TestDescriptorMigrationSuite"}

	extendedDescriptorMigration := migration.NewDescriptorMigration(log.WithFields(fields))
	err := extendedDescriptorMigration.Up(suite.tx)
	require.NoError(suite.T(), err)

	// Verify that the migration has populated correctly the extended descriptor, by
	// fetching extended descriptor and unmarshalling it into ExtendedDescriptor structure
	selectStatement := "select extended_descriptor from jobs"
	rows, err := suite.tx.Query(selectStatement)
	defer rows.Close()

	require.NoError(suite.T(), err)

	present := rows.Next()
	require.True(suite.T(), present)
	require.NoError(suite.T(), rows.Err())

	extendedDescriptorJSON := ""
	err = rows.Scan(&extendedDescriptorJSON)
	require.NoError(suite.T(), err)

	extendedDescriptor := job.ExtendedDescriptor{}
	err = json.Unmarshal([]byte(extendedDescriptorJSON), &extendedDescriptor)
	require.NoError(suite.T(), err)

	stepDescriptors := extendedDescriptor.TestStepsDescriptors
	require.Equal(suite.T(), 1, len(stepDescriptors), fmt.Sprintf("%d != 1", len(stepDescriptors)))

	require.Equal(suite.T(), extendedDescriptor.TestStepsDescriptors[0].TestName, "Literal test")

	test := extendedDescriptor.TestStepsDescriptors[0].TestSteps
	require.Equal(suite.T(), len(test), 1, fmt.Sprintf("%d!= 1", len(test)))
	require.Equal(suite.T(), "cmd", test[0].Name)
	require.Equal(suite.T(), "some label", test[0].Label)

}
func TestTestDescriptorMigrationSuite(t *testing.T) {
	testSuite := TestDescriptorMigrationSuite{}
	suite.Run(t, &testSuite)
}
