// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// Package inmemory implements an in-memory target locker.
// WARNING: since locking is done in memory, locally to the ConTest server, this
// will not prevent other ConTest servers to know that the target is in use.
package inmemory

import (
	"fmt"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

// Name is the name used to look this plugin up.
var Name = "InMemory"

type request struct {
	ctx     xcontext.Context
	targets []*target.Target
	// requireLocked specifies whether targets must be already locked, used by refresh.
	requireLocked bool
	// allowConflicts enabled tryLock semantics: Return what can be locked, but
	// don't error on conflicts
	allowConflicts bool
	// owner is the owner of the lock, relative to the lock operation request,
	// represented by a job ID.
	// The in-memory locker enforces that the requests are validated against the
	// right owner.
	owner types.JobID
	// limit provides an upper limit on how many locks will be acquired
	limit uint
	// timeout is the initial lock duration when acquiring a new lock
	// during target acquisition. This should include TargetManagerAcquireTimeout
	// to allow for dynamic locking in the target manager.
	timeout time.Duration
	// locked is list of target IDs that were locked in this transaction (if any)
	locked []string
	// err reports whether there were errors in any lock-related operation.
	err chan error
}

type lock struct {
	owner     types.JobID
	createdAt time.Time
	expiresAt time.Time
}

func validateRequest(req *request) error {
	if req == nil {
		return fmt.Errorf("got nil request")
	}
	if req.owner == 0 {
		return fmt.Errorf("owner cannot be zero")
	}
	for _, target := range req.targets {
		if target.ID == "" {
			return fmt.Errorf("target list cannot contain empty target ID. Full list: %v", req.targets)
		}
	}
	return nil
}

// broker is the broker of locking requests, and it's the only goroutine with
// access to the locks map, in accordance with Go's "share memory by
// communicating" principle.
func broker(clk clock.Clock, lockRequests, unlockRequests <-chan *request, done <-chan struct{}) {
	locks := make(map[string]lock)
	for {
		select {
		case <-done:
			return
		case req := <-lockRequests:
			if err := validateRequest(req); err != nil {
				req.err <- fmt.Errorf("lock request: %w", err)
				continue
			}
			var lockErr error
			// newLocks is the state of the locks that have been modified by this transaction
			// If there is an error, we discard newLocks leaving the state of 'locks' untouched
			// otherwise we update 'locks' with the modifed locks after the transaction has completed
			newLocks := make(map[string]lock)
			for _, t := range req.targets {
				// don't go over limit
				if uint(len(newLocks)) >= req.limit {
					break
				}
				now := clk.Now()
				if l, ok := locks[t.ID]; ok {
					// target has been locked before. are/were we the owner?
					// if so, extend (even if previous lease has expired).
					if l.owner == req.owner {
						// we are trying to extend a lock.
						l.expiresAt = clk.Now().Add(req.timeout)
						newLocks[t.ID] = l
					} else {
						// no, it's not us. can we take over?
						if now.After(l.expiresAt) {
							if !req.requireLocked {
								// lock has expired, consider it unlocked
								newLocks[t.ID] = lock{
									owner:     req.owner,
									createdAt: now,
									expiresAt: now.Add(req.timeout),
								}
							} else {
								lockErr = fmt.Errorf("target %q must be locked but isn't", t)
								break
							}
						} else {
							// already locked
							if !req.allowConflicts {
								lockErr = fmt.Errorf("target %q is already locked by %d", t, l.owner)
								break
							}
							continue
						}
					}
				} else {
					if !req.requireLocked {
						// target not locked and never been, create new lock
						newLocks[t.ID] = lock{
							owner:     req.owner,
							createdAt: now,
							expiresAt: now.Add(req.timeout),
						}
					} else {
						lockErr = fmt.Errorf("target %q must be locked but isn't", t)
						break
					}
				}
			}
			req.locked = make([]string, 0, len(newLocks))
			if lockErr == nil {
				// everything in this transaction was OK - update the locks
				for id, l := range newLocks {
					locks[id] = l
					req.locked = append(req.locked, id)
				}
			}
			req.err <- lockErr
		case req := <-unlockRequests:
			if err := validateRequest(req); err != nil {
				req.err <- fmt.Errorf("unlock request: %w", err)
				continue
			}
			req.ctx.Debugf("Requested to transactionally unlock %d targets: %v", len(req.targets), req.targets)
			// validate
			var unlockErr error
			for _, t := range req.targets {
				l, ok := locks[t.ID]
				if !ok {
					unlockErr = fmt.Errorf("unlock request: target %q is not locked", t)
					break
				}
				if l.owner != req.owner {
					unlockErr = fmt.Errorf("unlock request: target %q is locked by %d, not by %d", t, l.owner, req.owner)
					break
				}
			}
			// apply
			if unlockErr == nil {
				for _, t := range req.targets {
					delete(locks, t.ID)
				}
			}
			req.err <- unlockErr
		}
	}
}

// InMemory locks targets in an in-memory map.
type InMemory struct {
	lockRequests, unlockRequests chan *request
	done                         chan struct{}
}

func newReq(ctx xcontext.Context, jobID types.JobID, targets []*target.Target) request {
	return request{
		ctx:     ctx,
		targets: targets,
		owner:   jobID,
		err:     make(chan error),
	}
}

// Lock locks the specified targets.
func (tl *InMemory) Lock(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*target.Target) error {
	req := newReq(ctx, jobID, targets)
	req.timeout = duration
	req.requireLocked = false
	req.allowConflicts = false
	req.limit = uint(len(targets))
	tl.lockRequests <- &req
	err := <-req.err
	ctx.Debugf("Lock %d targets for %s: %v", len(targets), duration, err)
	return err
}

// Lock locks the specified targets.
func (tl *InMemory) TryLock(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*target.Target, limit uint) ([]string, error) {
	req := newReq(ctx, jobID, targets)
	req.timeout = duration
	req.requireLocked = false
	req.allowConflicts = true
	req.limit = limit
	tl.lockRequests <- &req
	// wait for result
	err := <-req.err
	ctx.Debugf("TryLock %d targets for %s: %d %v", len(targets), duration, len(req.locked), err)
	return req.locked, err
}

// Unlock unlocks the specified targets.
func (tl *InMemory) Unlock(ctx xcontext.Context, jobID types.JobID, targets []*target.Target) error {
	req := newReq(ctx, jobID, targets)
	tl.unlockRequests <- &req
	err := <-req.err
	ctx.Debugf("Unlock %d targets: %v", len(targets), err)
	return err
}

// RefreshLocks extends the lock duration by the internally configured timeout. If
// the owner is different, the request is rejected.
func (tl *InMemory) RefreshLocks(ctx xcontext.Context, jobID types.JobID, duration time.Duration, targets []*target.Target) error {
	req := newReq(ctx, jobID, targets)
	req.timeout = duration
	req.requireLocked = true
	req.allowConflicts = false
	req.limit = uint(len(targets))
	// refreshing a lock is just a lock operation with the same owner and a new
	// duration.
	tl.lockRequests <- &req
	err := <-req.err
	ctx.Debugf("RefreshLocks on %d targets for %s: %v", len(targets), duration, err)
	return err
}

// Close stops the brokern and releases resources.
func (tl *InMemory) Close() error {
	close(tl.done)
	return nil
}

// New initializes and returns a new InMemory target locker.
func New(clk clock.Clock) target.Locker {
	lockRequests := make(chan *request)
	unlockRequests := make(chan *request)
	done := make(chan struct{})
	go broker(clk, lockRequests, unlockRequests, done)
	return &InMemory{
		lockRequests:   lockRequests,
		unlockRequests: unlockRequests,
		done:           done,
	}
}
