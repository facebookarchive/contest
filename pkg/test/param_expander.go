// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package test

import (
	"fmt"
	"reflect"

	"github.com/facebookincubator/contest/pkg/target"
)

type ParamExpander struct {
	t *target.Target
}

func NewParamExpander(target *target.Target) *ParamExpander {
	return &ParamExpander{target}
}

func (pe *ParamExpander) Expand(value string) (string, error) {
	p := NewParam(value)
	return p.Expand(pe.t)
}

func (pe *ParamExpander) ExpandObject(obj interface{}, out interface{}) error {
	if reflect.PtrTo(reflect.TypeOf(obj)) != reflect.TypeOf(out) {
		return fmt.Errorf("object types differ")
	}

	var visit func(vin reflect.Value, vout reflect.Value) error
	visit = func(vin reflect.Value, vout reflect.Value) error {
		if !vout.CanSet() {
			return fmt.Errorf("readonly field: %v", vout.Type().Name())
		}

		switch vin.Kind() {
		case reflect.String:
			val, err := pe.Expand(vin.String())
			if err != nil {
				return err
			}
			vout.SetString(val)

		case reflect.Struct:
			for i := 0; i < vin.NumField(); i++ {
				if err := visit(vin.Field(i), vout.Field(i)); err != nil {
					return err
				}
			}

		case reflect.Array:
		case reflect.Slice:
			elemType := vin.Type().Elem()
			len := vin.Len()

			val := reflect.MakeSlice(reflect.SliceOf(elemType), len, len)
			vout.Set(val)

			for i := 0; i < len; i++ {
				if err := visit(vin.Index(i), vout.Index(i)); err != nil {
					return err
				}
			}

		default:
			vout.Set(vin)
		}
		return nil
	}

	vin := reflect.ValueOf(obj)
	vout := reflect.ValueOf(out)
	if vout.Kind() != reflect.Ptr {
		return fmt.Errorf("out must be a pointer type")
	}

	return visit(vin, reflect.Indirect(vout))
}
