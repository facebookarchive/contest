package xcontext

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"
)

// WithTimeout is analog of context.WithTimeout, but with support of the
// extended Context.
func WithTimeout(parent Context, timeout time.Duration) (Context, CancelFunc) {
	return WithDeadline(parent, time.Now().Add(timeout))
}

// WithDeadline is analog of context.WithDeadline, but with support of the
// extended Context..
func WithDeadline(parent Context, t time.Time) (Context, CancelFunc) {
	if parent == nil {
		parent = background
	}

	ctx := parent.Clone()
	h := ctx.addEventHandler()
	h.deadline = &t
	time.AfterFunc(time.Until(t), func() {
		h.cancel(DeadlineExceeded)
	})
	return ctx, func() {
		h.cancel(Canceled)
	}
}

// WithCancel is analog of context.WithCancel, but with support of the
// extended Context.
//
// If no errs are passed, then cancel is used.
func WithCancel(parent Context, errs ...error) (Context, CancelFunc) {
	if parent == nil {
		parent = background
	}

	ctx := parent.Clone()
	h := ctx.addEventHandler()
	return ctx, func() {
		h.cancel(errs...)
	}
}

// WithResetSignalers resets all signalers (cancelers and notifiers).
func WithResetSignalers(parent Context) Context {
	if parent == nil {
		parent = background
	}

	ctx := parent.Clone()
	ctx.resetEventHandler()
	return ctx
}

// WithNotify is analog WithCancel, but does a notification signal instead
// (which does not close the context).
//
// Panics if no errs are passed.
func WithNotify(parent Context, errs ...error) (Context, CancelFunc) {
	if len(errs) == 0 {
		panic("len(errs) == 0")
	}
	if parent == nil {
		parent = background
	}

	ctx := parent.Clone()
	h := ctx.addEventHandler()
	return ctx, func() {
		h.notify(errs...)
	}
}

// WithStdContext adds events and values of a standard context.
func WithStdContext(parent Context, stdCtx context.Context) Context {
	if parent == nil {
		parent = background
	}

	return parent.cloneWithStdContext(stdCtx)
}

func (ctx *ctxValue) cloneWithStdContext(stdCtx context.Context) Context {
	child := ctx.clone()

	if child.valuesHandler != nil {
		child.valuesHandler = valuesMerger{
			outer: stdCtx,
			inner: child.valuesHandler,
		}
	} else {
		child.valuesHandler = stdCtx
	}

	h := child.addEventHandler()

	child = child.clone()

	stopListening := make(chan struct{})
	runtime.SetFinalizer(child, func(ctx *ctxValue) {
		close(stopListening)
	})
	go func() {
		for {
			select {
			case <-stdCtx.Done():
				h.cancel(stdCtx.Err())
			case <-stopListening:
			}
			return
		}
	}()

	return child
}

type eventHandler struct {
	children map[*eventHandler]struct{}

	// locker is used exclude concurrent access to any data below (in this
	// structure).
	locker sync.Mutex

	// firstCancel is the first cancel signal ever received for the whole.
	// context tree.
	//
	// It is used to implement method Err compatible with the co-named method
	// of the original context.Context.
	firstCancel error

	// receivedCancels is only the cancel signals received by this node.
	receivedCancels []error

	// cancelSignal is closed when a new cancel signal is arrived
	cancelSignal chan struct{}

	// receivedNotifications same as receivedCancels, but for notification signals.
	receivedNotifications []error

	// notifySignal same as cancelSignal, but for notification signals.
	notifySignal map[error]chan struct{}

	// deadline is when the context will be closed by exceeding a timeout.
	//
	// If nil then never.
	deadline *time.Time
}

func (ctx *ctxValue) resetEventHandler() {
	ctx.eventHandler = nil
}

func (ctx *ctxValue) addEventHandler() *eventHandler {
	parent := ctx.eventHandler
	h := &eventHandler{
		notifySignal: make(map[error]chan struct{}),
		children:     map[*eventHandler]struct{}{},
	}
	ctx.eventHandler = h

	if parent == nil {
		return h
	}

	parent.locker.Lock()
	h.firstCancel = parent.firstCancel
	h.receivedNotifications = make([]error, len(parent.receivedNotifications))
	copy(h.receivedNotifications, parent.receivedNotifications)
	h.receivedCancels = make([]error, len(parent.receivedCancels))
	copy(h.receivedCancels, parent.receivedCancels)
	parent.children[h] = struct{}{}
	parent.locker.Unlock()

	runtime.SetFinalizer(ctx, func(ctx *ctxValue) {
		parent.locker.Lock()
		delete(parent.children, h)
		parent.locker.Unlock()
	})

	return h
}

// IsSignaledWith returns true if the context received a cancel
// or a notification signal equals to any of passed ones.
//
// If errs is empty, then returns true if the context received any
// cancel or notification signal.
func (h *eventHandler) IsSignaledWith(errs ...error) bool {
	if h == nil {
		return false
	}

	h.locker.Lock()
	defer h.locker.Unlock()

	return h.isSignaledWith(errs...)
}

func (h *eventHandler) isSignaledWith(errs ...error) bool {
	if len(errs) == 0 && (len(h.receivedCancels) != 0 || len(h.receivedNotifications) != 0) {
		return true
	}

	for _, receivedErr := range h.receivedCancels {
		for _, err := range errs {
			if errors.Is(receivedErr, err) {
				return true
			}
		}
	}

	for _, receivedErr := range h.receivedNotifications {
		for _, err := range errs {
			if errors.Is(receivedErr, err) {
				return true
			}
		}
	}

	return false
}

// Err implements context.Context.Err
func (h *eventHandler) Err() error {
	if h == nil {
		return nil
	}

	h.locker.Lock()
	defer h.locker.Unlock()
	if h.firstCancel != nil {
		return h.firstCancel
	}
	return nil
}

func (h *eventHandler) err() error {
	h.locker.Lock()
	defer h.locker.Unlock()

	if len(h.receivedCancels) != 0 {
		return h.receivedCancels[0]
	}

	return nil
}

func (h *eventHandler) cancel(errs ...error) {
	h.locker.Lock()
	defer h.locker.Unlock()

	if len(errs) == 0 {
		h.receivedCancels = append(h.receivedCancels, Canceled)
	} else {
		h.receivedCancels = append(h.receivedCancels, errs...)
	}
	if h.firstCancel == nil {
		h.firstCancel = h.receivedCancels[0]
	}

	if h.cancelSignal != nil {
		cancelSignal := h.cancelSignal
		h.cancelSignal = nil
		close(cancelSignal)
	}

	for _, err := range errs {
		if h.notifySignal[err] == nil {
			continue
		}
		close(h.notifySignal[err])
		h.notifySignal[err] = nil
	}

	if h.notifySignal[nil] != nil {
		close(h.notifySignal[nil])
		h.notifySignal[nil] = nil
	}

	for child := range h.children {
		child.cancel(errs...)
	}
}

func (h *eventHandler) notify(errs ...error) {
	h.locker.Lock()
	defer h.locker.Unlock()

	h.receivedNotifications = append(h.receivedNotifications, errs...)

	for _, err := range errs {
		if h.notifySignal[err] == nil {
			continue
		}
		close(h.notifySignal[err])
		h.notifySignal[err] = nil
	}

	if h.notifySignal[nil] != nil {
		close(h.notifySignal[nil])
		h.notifySignal[nil] = nil
	}

	for child := range h.children {
		child.notify(errs...)
	}
}

// Notifications returns all the received notifications (including events
// received by parents).
//
// This is a read-only value, do not modify it.
func (h *eventHandler) Notifications() []error {
	if h == nil {
		return nil
	}

	h.locker.Lock()
	defer h.locker.Unlock()

	return h.receivedNotifications
}

var (
	openChan   = make(chan struct{})
	closedChan = make(chan struct{})
)

func init() {
	close(closedChan)
}

// Done implements context.Context.Done
func (h *eventHandler) Done() <-chan struct{} {
	if h == nil {
		return nil
	}

	h.locker.Lock()
	defer h.locker.Unlock()

	if h.firstCancel != nil {
		return closedChan
	}

	if h.cancelSignal == nil {
		h.cancelSignal = make(chan struct{})
	}
	return h.cancelSignal
}

// Until works similar to Done(), but it is possible to specify specific
// signal to wait for.
//
// If err is nil, then waits for any event.
func (h *eventHandler) Until(err error) <-chan struct{} {
	if h == nil {
		return openChan
	}
	return h.newWaiter(err)
}

// Deadline implements context.Context.Deadline
func (h *eventHandler) Deadline() (deadline time.Time, ok bool) {
	if h == nil {
		return
	}

	h.locker.Lock()
	defer h.locker.Unlock()

	if h.deadline == nil {
		return
	}

	return *h.deadline, true
}

func (h *eventHandler) newWaiter(err error) <-chan struct{} {
	h.locker.Lock() // is unlocked in the go func() below
	defer h.locker.Unlock()
	if (err == nil && h.isSignaledWith()) || (err != nil && h.isSignaledWith(err)) {
		return closedChan
	}

	if h.notifySignal[err] == nil {
		h.notifySignal[err] = make(chan struct{})
	}
	return h.notifySignal[err]
}
