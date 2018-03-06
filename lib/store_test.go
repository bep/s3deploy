// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkStrings(t *testing.T) {
	assert := require.New(t)

	c1 := chunkStrings([]string{"a", "b", "c", "d"}, 2)
	c2 := chunkStrings([]string{"a", "b", "c", "d"}, 3)
	c3 := chunkStrings([]string{}, 2)
	assert.Equal([][]string{{"a", "b"}, {"c", "d"}}, c1)
	assert.Equal([][]string{{"a", "b", "c"}, {"d"}}, c2)
	assert.Equal(0, len(c3))
}

func TestNoUpdateStore(t *testing.T) {
	store := new(noUpdateStore)
	assert := require.New(t)
	m, err := store.FileMap()
	assert.NoError(err)
	assert.Equal(0, len(m))
	assert.NoError(store.DeleteObjects(context.Background(), nil))
	assert.NoError(store.Put(context.Background(), nil))
}
