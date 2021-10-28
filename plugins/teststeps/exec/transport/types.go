// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package transport

import (
	"fmt"
	"os"
	"sync"
	"syscall"
)

type deferedStack struct {
	funcs []func()

	closed bool
	done   chan struct{}

	mu sync.Mutex
}

func newDeferedStack() *deferedStack {
	s := &deferedStack{nil, false, make(chan struct{}), sync.Mutex{}}

	go func() {
		<-s.done
		for i := len(s.funcs) - 1; i >= 0; i-- {
			s.funcs[i]()
		}
	}()

	return s
}

func (s *deferedStack) Add(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.funcs = append(s.funcs, f)
}

func (s *deferedStack) Done() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	close(s.done)
	s.closed = true
}

func canExecute(fi os.FileInfo) bool {
	// TODO: deal with acls?
	stat := fi.Sys().(*syscall.Stat_t)
	if stat.Uid == uint32(os.Getuid()) {
		return stat.Mode&0500 == 0500
	}

	if stat.Gid == uint32(os.Getgid()) {
		return stat.Mode&0050 == 0050
	}

	return stat.Mode&0005 == 0005
}

func checkBinary(bin string) error {
	// check binary exists and is executable
	fi, err := os.Stat(bin)
	if err != nil {
		return fmt.Errorf("no such file")
	}

	if !fi.Mode().IsRegular() {
		return fmt.Errorf("not a file")
	}

	if !canExecute(fi) {
		return fmt.Errorf("provided binary is not executable")
	}
	return nil
}
