// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"context"
	"sync"
	"time"
)

// Note: unfortunately context.WithCancel can't redefine error code

func newCancelContext(ctx context.Context) (context.Context, func(err error)) {
	result := &cancelContext{
		done: make(chan struct{}),
	}
	result.propagateCancel(ctx)
	return result, func(err error) {
		result.cancel(err)
	}
}

type cancelContext struct {
	mu   sync.Mutex
	err  error
	done chan struct{}

	parent   *cancelContext
	children map[*cancelContext]struct{} // set to nil by the first cancel call
}

func (*cancelContext) Deadline() (deadline time.Time, ok bool) {
	return
}

func (c *cancelContext) Value( /*key*/ interface{}) interface{} {
	return nil
}

func (c *cancelContext) Done() <-chan struct{} {
	return c.done
}

func (c *cancelContext) Err() error {
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	return err
}

func (c *cancelContext) cancel(err error) {
	internalRelease := func(err error) (*cancelContext, []*cancelContext) {
		c.mu.Lock()
		defer c.mu.Unlock()

		select {
		case <-c.done:
			return nil, nil // already canceled
		default:
		}

		children := make([]*cancelContext, 0, len(c.children))
		for c := range c.children {
			children = append(children, c)
		}
		c.children = nil

		parent := c.parent
		c.parent = nil

		c.err = err
		close(c.done)

		return parent, children
	}

	parent, children := internalRelease(err)

	// unsubscribe from parent
	if parent != nil {
		func() {
			parent.mu.Lock()
			defer parent.mu.Unlock()

			if parent.children == nil {
				return
			}
			delete(parent.children, c)
		}()
	}

	// propagate cancel to children
	for _, ch := range children {
		ch.cancel(err)
	}
}

func (c *cancelContext) propagateCancel(parent context.Context) {
	done := parent.Done()
	if done == nil {
		return // parent is never canceled
	}

	// Creating extra-goroutines is bad because it requires either invoking cancel by parent or child context
	if cc, ok := parent.(*cancelContext); ok {
		c.parent = cc
		subscribe := func() bool {
			cc.mu.Lock()
			defer cc.mu.Unlock()

			select {
			case <-cc.done:
				// parent is already canceled
				return false
			default:
			}

			if cc.children == nil {
				cc.children = make(map[*cancelContext]struct{})
			}
			cc.children[c] = struct{}{}
			return true
		}

		if !subscribe() {
			c.cancel(parent.Err())
		}
		return
	}

	select {
	case <-done:
		// parent is already canceled
		c.cancel(parent.Err())
		return
	default:
	}

	go func() {
		select {
		case <-parent.Done():
			c.cancel(parent.Err())
		case <-c.Done():
		}
	}()
}
