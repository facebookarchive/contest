// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package xcontext

import (
	"sync/atomic"
	"unsafe"
)

func (ctx *ctxValue) loadDebugTools() *debugTools {
	return (*debugTools)(atomic.LoadPointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.debugTools))))
}

func (ctx *ctxValue) storeDebugTools(newDebugTools *debugTools) {
	atomic.StorePointer((*unsafe.Pointer)((unsafe.Pointer)(&ctx.debugTools)), unsafe.Pointer(newDebugTools))
}
