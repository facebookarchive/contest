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

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

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

// broker is the broker of locking requests, and it's the only goroutine with
// access to the locks map, in accordance with Go's "share memory by
// communicating" principle.
func broker(lockRequests, unlockRequests, checkLocksRequests <-chan request, done <-chan struct{}) {
	locks := make(map[target.Target]lock)
	for {
		select {
		case <-done:
			log.Debugf("Shutting down in-memory target locker")
			return
		case req := <-lockRequests:
			log.Debugf("Requested to lock %d targets: %v", len(req.targets), req.targets)
			var lockErr error
			for _, t := range req.targets {
				now := time.Now()
				if l, ok := locks[*t]; ok {
					// target has been locked before. Is it still locked, or did
					// it expire?
					if now.After(l.expiresAt) {
						// lock has expired, consider it unlocked
						locks[*t] = lock{
							owner:     req.owner,
							lockedAt:  now,
							expiresAt: now.Add(req.timeout),
						}
					} else {
						// target is locked. Is it us or someone else?
						if l.owner == req.owner {
							// we are trying to extend a lock.
							l := locks[*t]
							l.expiresAt = time.Now().Add(req.timeout)
							locks[*t] = l
						} else {
							lockErr = fmt.Errorf("target already locked: %+v", t)
						}
						break
					}
				} else {
					// target not locked and never seen, create new lock
					locks[*t] = lock{
						owner:     req.owner,
						lockedAt:  now,
						expiresAt: now.Add(req.timeout),
					}
				}
			}
			req.err <- lockErr
		case req := <-unlockRequests:
			log.Debugf("Requested to transactionally unlock %d targets: %v", len(req.targets), req.targets)
			for _, t := range req.targets {
				if _, ok := locks[*t]; ok {
					delete(locks, *t)
				} else {
					log.Debugf("Target is not locked, but received unlock request: %+v", t)
				}
			}
			req.err <- nil
		case req := <-checkLocksRequests:
			log.Debugf("Requested to check locks for %d targets: %v", len(req.targets), req.targets)
			locked := make([]*target.Target, 0)
			notLocked := make([]*target.Target, 0)
			for _, t := range req.targets {
				if l, ok := locks[*t]; ok {
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

// InMemory is the no-op target locker. It does nothing.
type InMemory struct {
	lockRequests, unlockRequests, checkLocksRequests chan request
	done                                             chan struct{}
	timeout                                          time.Duration
}

func newReq(targets []*target.Target) request {
	return request{
		targets: targets,
		err:     make(chan error),
	}
}

// Lock locks the specified targets by doing nothing.
func (tl *InMemory) Lock(jobID types.JobID, targets []*target.Target) error {
	log.Infof("Trying to lock %d targets", len(targets))
	req := newReq(targets)
	req.timeout = tl.timeout
	tl.lockRequests <- req
	return <-req.err
}

// Unlock unlocks the specified targets by doing nothing.
func (tl *InMemory) Unlock(jobID types.JobID, targets []*target.Target) error {
	log.Infof("Trying to unlock %d targets", len(targets))
	req := newReq(targets)
	tl.unlockRequests <- req
	return <-req.err
}

// RefreshLocks extends the lock duration by the internally configured timeout. If
// the owner is different, the request is rejected.
func (tl *InMemory) RefreshLocks(jobID types.JobID, targets []*target.Target) error {
	log.Infof("Trying to refresh locks on %d targets by %s", len(targets), tl.timeout)
	req := newReq(targets)
	req.timeout = tl.timeout
	// refreshing a lock is just a lock operation with the same owner and a new
	// duration.
	tl.lockRequests <- req
	return <-req.err
}

// CheckLocks tells whether all the targets are locked. They all are, always. It
// also returns the ones that are not locked.
func (tl *InMemory) CheckLocks(jobID types.JobID, targets []*target.Target) (bool, []*target.Target, []*target.Target) {
	log.Infof("Checking if %d target(s) are locked by job ID %d", len(targets), jobID)
	req := newReq(targets)
	tl.checkLocksRequests <- req
	if err := <-req.err; err != nil {
		// TODO the target.Locker.CheckLocks interface has to change to return an
		// error as well, because lock checking can fail, e.g. when using
		// external services.
		// Just log an error for now.
		log.Warningf("Error when trying to unlock targets: %v", err)
	}
	return len(req.notLocked) != len(targets), req.locked, req.notLocked
}

// New initializes and returns a new ExampleTestStep.
func New(timeout time.Duration) target.Locker {
	lockRequests := make(chan request)
	unlockRequests := make(chan request)
	checkLocksRequests := make(chan request)
	done := make(chan struct{}, 1)
	go broker(lockRequests, unlockRequests, checkLocksRequests, done)
	return &InMemory{
		lockRequests:       lockRequests,
		unlockRequests:     unlockRequests,
		checkLocksRequests: checkLocksRequests,
		done:               done,
		timeout:            timeout,
	}
}
