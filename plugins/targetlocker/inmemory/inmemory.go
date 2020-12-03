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
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/types"
)

// Name is the name used to look this plugin up.
var Name = "InMemory"

var log = logging.GetLogger("targetlocker/" + strings.ToLower(Name))

type request struct {
	targets []*target.Target
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
	// timeout is how long the lock should be held for. There is no lower or
	// upper bound on how long the lock can be held.
	timeout time.Duration
	// locked is list of target IDs that were locked in this transaction (if any)
	locked []string
	// err reports whether there were errors in any lock-related operation.
	err chan error
}

type lock struct {
	owner     types.JobID
	lockedAt  time.Time
	expiresAt time.Time
}

func validateRequest(req *request) error {
	if req == nil {
		return fmt.Errorf("got nil request")
	}
	if req.owner == 0 {
		return fmt.Errorf("owner cannot be zero")
	}
	if len(req.targets) == 0 {
		return fmt.Errorf("no target specified")
	}
	return nil
}

// broker is the broker of locking requests, and it's the only goroutine with
// access to the locks map, in accordance with Go's "share memory by
// communicating" principle.
func broker(lockRequests, unlockRequests <-chan *request, done <-chan struct{}) {
	locks := make(map[target.Target]lock)
	for {
		select {
		case <-done:
			log.Debugf("Shutting down in-memory target locker")
			return
		case req := <-lockRequests:
			if err := validateRequest(req); err != nil {
				req.err <- fmt.Errorf("lock request: %w", err)
				continue
			}
			log.Debugf("Requested to lock %d targets for job ID %d: %v", len(req.targets), req.owner, req.targets)
			var lockErr error
			// newLocks is the state of the locks that have been modified by this transaction
			// If there is an error, we discard newLocks leaving the state of 'locks' untouched
			// otherwise we update 'locks' with the modifed locks after the transaction has completed
			newLocks := make(map[target.Target]lock)
			for _, t := range req.targets {
				// don't go over limit
				if uint(len(newLocks)) >= req.limit {
					break
				}
				now := time.Now()

				if l, ok := locks[*t]; ok {
					// target has been locked before. Is it still locked, or did
					// it expire?
					if now.After(l.expiresAt) {
						// lock has expired, consider it unlocked
						newLocks[*t] = lock{
							owner:     req.owner,
							lockedAt:  now,
							expiresAt: now.Add(req.timeout),
						}
					} else {
						// target is locked. Is it us or someone else?
						if l.owner == req.owner {
							// we are trying to extend a lock.
							l.expiresAt = time.Now().Add(req.timeout)
							newLocks[*t] = l
						} else {
							// already locked
							if !req.allowConflicts {
								lockErr = fmt.Errorf("lock request: target already locked: %+v (lock: %+v)", t, l)
								break
							}
							continue
						}
					}
				} else {
					// target not locked and never seen, create new lock
					newLocks[*t] = lock{
						owner:     req.owner,
						lockedAt:  now,
						expiresAt: now.Add(req.timeout),
					}
				}
			}
			req.locked = make([]string, 0, len(newLocks))
			if lockErr == nil {
				// everything in this transaction was OK - update the locks
				for t, l := range newLocks {
					locks[t] = l
					req.locked = append(req.locked, t.ID)
				}
			}
			req.err <- lockErr
		case req := <-unlockRequests:
			if err := validateRequest(req); err != nil {
				req.err <- fmt.Errorf("unlock request: %w", err)
				continue
			}
			log.Debugf("Requested to transactionally unlock %d targets: %v", len(req.targets), req.targets)
			var unlockErr error
			for _, t := range req.targets {
				if l, ok := locks[*t]; ok {
					if l.owner == req.owner {
						delete(locks, *t)
					} else {
						unlockErr = fmt.Errorf("unlock request: denying unlock request from job ID %d on lock owned by job ID %d", req.owner, l.owner)
					}
				} else {
					// XXX should we roll back the unlock if a target fails?
					unlockErr = fmt.Errorf("unlock request: target is not locked: %+v", t)
					break
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
	// lockTimeout set on each initial lock request
	lockTimeout time.Duration
	// refreshTimeout is used during refresh
	refreshTimeout time.Duration
}

func newReq(jobID types.JobID, targets []*target.Target) request {
	return request{
		targets: targets,
		owner:   jobID,
		err:     make(chan error),
	}
}

// Lock locks the specified targets.
func (tl *InMemory) Lock(jobID types.JobID, targets []*target.Target) error {
	log.Infof("Trying to lock %d targets", len(targets))
	req := newReq(jobID, targets)
	req.timeout = tl.lockTimeout
	req.limit = uint(len(targets))
	tl.lockRequests <- &req
	return <-req.err
}

// Lock locks the specified targets.
func (tl *InMemory) TryLock(jobID types.JobID, targets []*target.Target, limit uint) ([]string, error) {
	log.Infof("Trying to trylock on %d targets", len(targets))
	req := newReq(jobID, targets)
	req.timeout = tl.lockTimeout
	req.allowConflicts = true
	req.limit = limit
	tl.lockRequests <- &req
	// wait for result
	err := <-req.err
	return req.locked, err
}

// Unlock unlocks the specified targets.
func (tl *InMemory) Unlock(jobID types.JobID, targets []*target.Target) error {
	log.Infof("Trying to unlock %d targets", len(targets))
	req := newReq(jobID, targets)
	tl.unlockRequests <- &req
	return <-req.err
}

// RefreshLocks extends the lock duration by the internally configured timeout. If
// the owner is different, the request is rejected.
func (tl *InMemory) RefreshLocks(jobID types.JobID, targets []*target.Target) error {
	log.Infof("Trying to refresh locks on %d targets by %s", len(targets), tl.refreshTimeout)
	req := newReq(jobID, targets)
	req.timeout = tl.refreshTimeout
	req.limit = uint(len(targets))
	// refreshing a lock is just a lock operation with the same owner and a new
	// duration.
	tl.lockRequests <- &req
	return <-req.err
}

// New initializes and returns a new InMemory target locker.
func New(lockTimeout, refreshTimeout time.Duration) target.Locker {
	lockRequests := make(chan *request)
	unlockRequests := make(chan *request)
	done := make(chan struct{}, 1)
	go broker(lockRequests, unlockRequests, done)
	return &InMemory{
		lockRequests:   lockRequests,
		unlockRequests: unlockRequests,
		done:           done,
		lockTimeout:    lockTimeout,
		refreshTimeout: refreshTimeout,
	}
}
