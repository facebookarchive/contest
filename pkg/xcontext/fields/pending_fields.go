// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package fields

// PendingField is a one entry of a history of added fields.
//
// PendingField is focused on high-performance Clone method, for details
// see PendingFields.
type PendingField struct {
	parentPendingFields Slice

	// In current implementation oneFieldKey and multipleFields are never
	// used together, but still we preserve oneFieldKey and oneFieldValue due
	// to performance reasons (see the description of PendingFields), it
	// allows to avoid allocating and accessing a map for a single field.
	oneFieldKey    string
	oneFieldValue  interface{}
	multipleFields map[string]interface{}
}

// Slice is a set of entries of a history of added fields.
//
// See PendingFields.
type Slice []PendingField

// PendingFields is a history of added fields. It is focused on cheap
// cloning and adding new entries avoiding extra copy all the data on each
// adding of key-values. Therefore it is so-so performance-safe to
// use it for scoped contexts.
//
// A resulting value could be received (compiled) on-need-basis using method Compile.
//
// See also benchmark comments in pending_fields_test.go.
type PendingFields struct {
	Slice      Slice
	IsReadOnly bool
}

// Clone returns a "copy" of PendingFields. It is safe to add new entries
// to this "copy" (it will not affect the original).
func (pendingFields PendingFields) Clone() PendingFields {
	if pendingFields.Slice != nil {
		pendingFields.IsReadOnly = true
	}
	return pendingFields
}

// Compile constructs Fields from PendingFields.
func (pendingFields PendingFields) Compile() Fields {
	return pendingFields.Slice.compile(nil)
}

// CompileWithStorage is the same as Compile, but allows to pass a storage
// to save values to. It could be used, for example, to reuse existing storages
// to reduce pressure on GC.
func (pendingFields PendingFields) CompileWithStorage(m Fields) Fields {
	return pendingFields.Slice.compile(m)
}

func (pendingFields Slice) compile(result Fields) Fields {
	for _, item := range pendingFields {
		switch {
		case item.parentPendingFields != nil:
			if result == nil {
				result = make(Fields, len(pendingFields)+len(item.parentPendingFields)-1)
			}

			item.parentPendingFields.compile(result)

		case item.multipleFields != nil:
			if result == nil {
				result = item.multipleFields
				continue
			}
			for k, v := range item.multipleFields {
				result[k] = v
			}

		default:
			if result == nil {
				result = make(Fields, len(pendingFields))
			}
			result[item.oneFieldKey] = item.oneFieldValue
		}
	}

	return result
}

// AddOne adds one field.
func (pendingFields *PendingFields) AddOne(key string, value interface{}) {
	if pendingFields.IsReadOnly {
		newPendingFields := PendingFields{Slice: make([]PendingField, 1, 2)}
		newPendingFields.Slice[0] = PendingField{parentPendingFields: pendingFields.Slice}
		*pendingFields = newPendingFields
	}
	pendingFields.Slice = append(pendingFields.Slice, PendingField{
		oneFieldKey:   key,
		oneFieldValue: value,
	})
}

// AddMultiple adds a set of fields.
func (pendingFields *PendingFields) AddMultiple(fields Fields) {
	if fields == nil {
		return
	}
	if pendingFields.IsReadOnly {
		newPendingFields := PendingFields{Slice: make([]PendingField, 1, 2)}
		newPendingFields.Slice[0] = PendingField{parentPendingFields: pendingFields.Slice}
		*pendingFields = newPendingFields
	}
	pendingFields.Slice = append(pendingFields.Slice, PendingField{
		multipleFields: fields,
	})
}
