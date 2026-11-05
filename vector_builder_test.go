// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

func TestVectorArgBeforeSQL(t *testing.T) {
	_, err := compile(dialect.For(dialect.SQLite), func(b *Builder) { b.Arg(vector.HalfVector{1}) })
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("halfvec on sqlite: err = %v, want ErrUnsupported", err)
	}
	c, err := compile(dialect.For(dialect.SQLite), func(b *Builder) { b.Arg(vector.Bits{Len: 8, Data: []byte{0x80}}) })
	if err != nil {
		t.Fatalf("bits on sqlite must compile: %v", err)
	}
	if len(c.Args) != 1 {
		t.Fatalf("args = %v, want one bound bit blob", c.Args)
	}
}
