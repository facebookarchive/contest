// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package targetlocker

// Action is a enumeration of ways to manipulate a target lock
type Action int

const (
	// ActionUndefined is the zero-value, therefore means "undefined".
	ActionUndefined = Action(iota)

	// ActionLock means it's about target.Locker.Lock()
	ActionLock

	// ActionUnlock means it's about target.Locker.Unlock()
	ActionUnlock

	// ActionRefreshLock means it's about target.Locker.RefreshLock()
	ActionRefreshLock
)

func (action Action) String() string {
	switch action {
	case ActionUndefined:
		return `undefined`
	case ActionLock:
		return `lock`
	case ActionUnlock:
		return `unlock`
	case ActionRefreshLock:
		return `refresh_locks`
	}
	return `unknown`
}
