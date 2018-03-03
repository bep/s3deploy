// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkStrings(t *testing.T) {
	assert := require.New(t)

	c1 := chunkStrings([]string{"a", "b", "c", "d"}, 2)
	c2 := chunkStrings([]string{"a", "b", "c", "d"}, 3)
	c3 := chunkStrings([]string{}, 2)
	assert.Equal([][]string{[]string{"a", "b"}, []string{"c", "d"}}, c1)
	assert.Equal([][]string{[]string{"a", "b", "c"}, []string{"d"}}, c2)
	assert.Equal(0, len(c3))

}
