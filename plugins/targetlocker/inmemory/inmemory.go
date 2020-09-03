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
	// owner is the owner of the lock, relative to the lock operation request,
	// represented by a job ID.
	// The in-memory locker enforces that the requests are validated against the
	// right owner.
	owner types.JobID
	// timeout is how long the lock should be held for. There is no lower or
	// upper bound on how long the lock can be held.
	timeout time.Duration
	// locked and notLocked are arrays of targets that are respectively already locked
	// by a given job ID, and that are not locked by a given job ID. This is only
	// populated when checking locks for such targets.
	locked, notLocked []*target.Target
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
func broker(lockRequests, unlockRequests, checkLocksRequests <-chan *request, done <-chan struct{}) {
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
							lockErr = fmt.Errorf("lock request: target already locked: %+v (lock: %+v)", t, l)
							break
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
			if lockErr == nil {
				// everything in this transaction was OK - update the locks
				for t, l := range newLocks {
					locks[t] = l
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
		case req := <-checkLocksRequests:
			if err := validateRequest(req); err != nil {
				req.err <- fmt.Errorf("checklocks request: %w", err)
				continue
			}
			log.Debugf("Requested to check locks for %d targets by job ID %d: %v", len(req.targets), req.owner, req.targets)
			locked := make([]*target.Target, 0)
			notLocked := make([]*target.Target, 0)
			for _, t := range req.targets {
				if l, ok := locks[*t]; ok {
					if l.owner == req.owner {
						now := time.Now()
						if now.After(l.expiresAt) {
							// target was locked but lock expired, purge the entry
							log.Debugf("Purged expired lock for target %+v. Lock time is %s, expiration timeout is %s", t, l.lockedAt, req.timeout)
							delete(locks, *t)
							notLocked = append(notLocked, t)
						} else {
							// target is locked
							locked = append(locked, t)
						}
					} else {
						// target is locked by someone else
						notLocked = append(notLocked, t)
					}
				} else {
					// target is not locked
					notLocked = append(notLocked, t)
				}
			}
			req.locked = locked
			req.notLocked = notLocked
			req.err <- nil
		}
	}
}

// InMemory locks targets in an in-memory map.
type InMemory struct {
	lockRequests, unlockRequests, checkLocksRequests chan *request
	done                                             chan struct{}
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
	tl.lockRequests <- &req
	return <-req.err
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
	// refreshing a lock is just a lock operation with the same owner and a new
	// duration.
	tl.lockRequests <- &req
	return <-req.err
}

// New initializes and returns a new InMemory target locker.
func New(lockTimeout, refreshTimeout time.Duration) target.Locker {
	lockRequests := make(chan *request)
	unlockRequests := make(chan *request)
	checkLocksRequests := make(chan *request)
	done := make(chan struct{}, 1)
	go broker(lockRequests, unlockRequests, checkLocksRequests, done)
	return &InMemory{
		lockRequests:       lockRequests,
		unlockRequests:     unlockRequests,
		checkLocksRequests: checkLocksRequests,
		done:               done,
		lockTimeout:        lockTimeout,
		refreshTimeout:     refreshTimeout,
	}
}
