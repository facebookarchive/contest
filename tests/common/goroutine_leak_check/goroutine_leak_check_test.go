// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package goroutine_leak_check

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Func1(c *sync.Cond) {
	c.L.Lock()
	c.Signal()
	c.Wait()
	c.L.Unlock()
}

func TestNoLeaks(t *testing.T) {
	require.NoError(t, CheckLeakedGoRoutines())
}

func TestLeaks(t *testing.T) {
	c := sync.NewCond(&sync.Mutex{})
	c.L.Lock()
	go Func1(c)
	c.Wait() // Wait for the go routine to start.
	c.L.Unlock()
	dump, err := checkLeakedGoRoutines()
	require.Errorf(t, err, "===\n%s\n===", dump)
	require.Contains(t, err.Error(), "goroutine_leak_check_test.go")
	require.Contains(t, err.Error(), "goroutine_leak_check.Func1")
	c.Signal()
	// Give some time for the goroutine to exit
	// otherwise it causes flakes in case of multiple test runs (-count=N).
	time.Sleep(10 * time.Millisecond)
}

func TestLeaksWhitelisted(t *testing.T) {
	c := sync.NewCond(&sync.Mutex{})
	c.L.Lock()
	go Func1(c)
	c.Wait()
	c.L.Unlock()
	require.NoError(t, CheckLeakedGoRoutines(
		"github.com/facebookincubator/contest/tests/common/goroutine_leak_check.Func1"))
	c.Signal()
	time.Sleep(10 * time.Millisecond)
}
