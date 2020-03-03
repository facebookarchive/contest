// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package abstract_test

import (
	"testing"

	. "github.com/facebookincubator/contest/pkg/abstract"
	"github.com/stretchr/testify/assert"
)

type factory struct{ Name string }

func (f factory) UniqueImplementationName() string { return f.Name }

func TestFactories_Find(t *testing.T) {
	factories := Factories{factory{Name: "c"}, factory{Name: "a"}, factory{Name: "b"}}

	r, _ := factories.Find("a").(factory)
	assert.Equal(t, "a", r.Name)

	r, ok := factories.Find("d").(factory)
	assert.False(t, ok)
	assert.Empty(t, r)
}
