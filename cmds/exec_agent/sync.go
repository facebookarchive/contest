// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import "sync"

// SafeSignal is a goroutine safe signalling mechanism
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
