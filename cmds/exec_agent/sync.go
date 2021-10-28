// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"sync"
)

// SafeSignal is a goroutine safe signalling mechanism
// It can have multiple goroutines that trigger the signal without
// interfering with eachother (or crashing as using a channel would)
// Currently designed for a single waiter goroutine.
type SafeSignal struct {
	sync.Mutex
	done bool
	c    *sync.Cond
}

func newSafeSignal() *SafeSignal {
	s := &SafeSignal{}
	s.c = &sync.Cond{L: s}
	return s
}

func (s *SafeSignal) Signal() {
	s.Lock()
	s.done = true
	s.Unlock()

	s.c.Signal()
}

func (s *SafeSignal) Wait() {
	s.Lock()
	defer s.Unlock()

	for !s.done {
		s.c.Wait()
	}
}

type SafeBuffer struct {
	b  bytes.Buffer
	mu sync.Mutex
}

func (sb *SafeBuffer) Write(data []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	return sb.b.Write(data)
}

func (sb *SafeBuffer) Read(data []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	return sb.b.Read(data)
}

func (sb *SafeBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	return sb.b.Len()
}
