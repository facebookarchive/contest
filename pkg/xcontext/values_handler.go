package xcontext

import (
	"context"
)

type valuesHandler interface {
	Value(key interface{}) interface{}
}

var _ valuesHandler = &valuesCtx{}

type valuesCtx struct {
	parent valuesHandler
	key    interface{}
	value  interface{}
}

// WithValue is analog of of context.WithValue but for extended context.
func WithValue(parent context.Context, key, value interface{}) Context {
	if parent, ok := parent.(Context); ok {
		return withValue(parent, key, value)
	}

	return withValue(NewContext(parent, "", nil, nil, nil, nil, nil), key, value)
}

func withValue(parent Context, key, value interface{}) Context {
	ctx := parent.Clone()
	ctx.addValue(key, value)
	return ctx
}

func (ctx *ctxValue) addValue(key, value interface{}) {
	ctx.valuesHandler = &valuesCtx{
		parent: ctx.valuesHandler,
		key:    key,
		value:  value,
	}
}

func (h *valuesCtx) Value(key interface{}) interface{} {
	if h.key == key {
		return h.value
	}
	if h.parent != nil {
		return h.parent.Value(key)
	}
	return nil
}

type valuesMerger struct {
	outer valuesHandler
	inner valuesHandler
}

func (m valuesMerger) Value(key interface{}) interface{} {
	v := m.outer.Value(key)
	if v != nil {
		return v
	}

	return m.inner.Value(key)
}
