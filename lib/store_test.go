// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestChunkStrings(t *testing.T) {
	c := qt.New(t)

	c1 := chunkStrings([]string{"a", "b", "c", "d"}, 2)
	c2 := chunkStrings([]string{"a", "b", "c", "d"}, 3)
	c3 := chunkStrings([]string{}, 2)
	c.Assert(c1, qt.DeepEquals, [][]string{{"a", "b"}, {"c", "d"}})
	c.Assert(c2, qt.DeepEquals, [][]string{{"a", "b", "c"}, {"d"}})
	c.Assert(len(c3), qt.Equals, 0)
}

func TestNoUpdateStore(t *testing.T) {
	store := new(noUpdateStore)
	c := qt.New(t)
	m, err := store.FileMap(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(len(m), qt.Equals, 0)
	c.Assert(store.DeleteObjects(context.Background(), nil), qt.IsNil)
	c.Assert(store.Put(context.Background(), nil), qt.IsNil)
}
