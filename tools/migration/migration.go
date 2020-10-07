// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package migration

import (
	"bytes"
	"database/sql"
	"io"
	"io/ioutil"
	"sort"

	"github.com/golang-migrate/migrate/v4/source"

	"fmt"
)

// Progress represents the migration progress of the task
type Progress struct {
	Completed uint64
	Total     uint64
}

// Task represents a single migration task
type Task interface {
	Desc() string
	Version() uint

	// Up and Down return the schema
	Up() string
	Down() string

	MigrateData(db *sql.DB, terminate chan struct{}, progress chan *Progress) error
}

// Tasks implements source.Driver from "github.com/golang-migrate/migrate/v4/source",
// holding internally a list of migrations
type Tasks struct {
	tasks         map[uint]Task
	tasksVersions []uint
}

// NewTasks initializes a new Tasks object
func NewTasks() *Tasks {
	m := Tasks{}
	m.tasks = make(map[uint]Task)
	m.tasksVersions = make([]uint, 0)
	return &m
}

// Register registers a migration task within Task
func (m *Tasks) Register(task Task) error {
	if _, present := m.tasks[task.Version()]; present {
		return fmt.Errorf("migration task %d is already registered", task.Version())
	}
	m.tasks[task.Version()] = task

	m.tasksVersions = append(m.tasksVersions, task.Version())
	sort.Slice(m.tasksVersions, func(i, j int) bool { return m.tasksVersions[i] < m.tasksVersions[j] })

	return nil
}

// GetTask returns the migration task corresponding to `version`
func (m *Tasks) GetTask(version uint) Task {
	if _, ok := m.tasks[version]; !ok {
		return nil
	}
	return m.tasks[version]
}

// Open is a no-op for MigrationTasks driver, as all available migrations are
// pre-registered in code
func (m *Tasks) Open(_ string) (source.Driver, error) {
	return m, nil
}

// Close closes the current MigrationTasks
func (m *Tasks) Close() error {
	return nil
}

// First returns the first available migration task
func (m *Tasks) First() (uint, error) {
	if len(m.tasksVersions) == 0 {
		return 0, fmt.Errorf("no migrations registered")
	}
	v, ok := m.tasks[m.tasksVersions[0]]
	if ok {
		return m.tasksVersions[0], nil
	}
	return 0, fmt.Errorf("migration v%d not associated with any migration", v)
}

// Prev returns the previous migration task
func (m *Tasks) Prev(version uint) (uint, error) {

	for index, v := range m.tasksVersions {
		if v == version {
			if index == 0 {
				return 0, fmt.Errorf("no previous version for v%d", v)
			}
			return m.tasksVersions[index-1], nil
		}
	}
	return 0, fmt.Errorf("version v%d not found", version)
}

// Next returns the next migration task
func (m *Tasks) Next(version uint) (uint, error) {

	for index, v := range m.tasksVersions {
		if v == version {
			if index == len(m.tasksVersions)-1 {
				return 0, fmt.Errorf("no next version for v%d", v)
			}
			return m.tasksVersions[index+1], nil
		}
	}
	return 0, fmt.Errorf("version v%d not found", version)
}

// ReadUp returns the up migration for `version`
func (m *Tasks) ReadUp(version uint) (io.ReadCloser, string, error) {
	if _, ok := m.tasks[version]; !ok {
		return nil, "", fmt.Errorf("no migration registered for version v%d", version)
	}

	t := m.tasks[version]
	rc := ioutil.NopCloser(bytes.NewReader([]byte(t.Up())))
	return rc, t.Desc(), nil
}

// ReadDown returns the down migration for `version`
func (m *Tasks) ReadDown(version uint) (io.ReadCloser, string, error) {
	if _, ok := m.tasks[version]; !ok {
		return nil, "", fmt.Errorf("no migration registered for version v%d", version)
	}

	t := m.tasks[version]
	rc := ioutil.NopCloser(bytes.NewReader([]byte(t.Down())))
	return rc, t.Desc(), nil
}
