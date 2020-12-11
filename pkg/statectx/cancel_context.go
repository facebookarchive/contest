// Note: unfortunately context.WithCancel can't redefine error code

package statectx

import (
	"context"
	"sync"
	"time"
)

func newCancelContext() (context.Context, func(err error)) {
	result := &cancelContext{
		done: make(chan struct{}),
	}
	return result, func(err error) {
		result.cancel(err)
	}
}

type cancelContext struct {
	mu   sync.Mutex
	err  error
	done chan struct{}
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
	select {
	case <-c.done:
		return // already canceled
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.err = err
	close(c.done)
}
