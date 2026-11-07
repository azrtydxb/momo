// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

// TestHalfvecDecodeEveryPattern decodes every finite float16 pattern and
// checks the server agrees on the widened value and that encode reproduces the
// bytes.
func TestHalfvecDecodeEveryPattern(t *testing.T) {
	for h := 0; h < 0x10000; h++ {
		if h&0x7c00 == 0x7c00 {
			continue // inf and NaN
		}
		f := halfToFloat32(uint16(h))
		got, ok := float32ToHalf(f)
		if !ok || got != uint16(h) {
			t.Fatalf("pattern %#04x -> %v -> %#04x ok=%v", h, f, got, ok)
		}
	}
}

// TestSparsevecDecodeRejectsOutOfRange feeds hand-built binary values past the
// limits to the decoder.
func TestSparsevecDecodeRejectsOutOfRange(t *testing.T) {
	build := func(dim, nnz uint32, idx []uint32) []byte {
		b := binary.BigEndian.AppendUint32(nil, dim)
		b = binary.BigEndian.AppendUint32(b, nnz)
		b = binary.BigEndian.AppendUint32(b, 0)
		for _, i := range idx {
			b = binary.BigEndian.AppendUint32(b, i)
		}
		for range idx {
			b = binary.BigEndian.AppendUint32(b, math.Float32bits(1))
		}
		return b
	}
	over := make([]uint32, 16001)
	for i := range over {
		over[i] = uint32(i)
	}
	cases := map[string][]byte{
		"dim 1e9+1":      build(vector.MaxSparseDim+1, 0, nil),
		"dim 0":          build(0, 0, nil),
		"dim negative":   build(0xffffffff, 0, nil),
		"nnz 16001":      build(1_000_000, 16001, over),
		"index == dim":   build(5, 1, []uint32{5}),
		"index negative": build(5, 1, []uint32{0xffffffff}),
		"not increasing": build(5, 2, []uint32{2, 2}),
	}
	for name, b := range cases {
		if _, err := decodeBinary(kindSparsevec, b); !errors.Is(err, vector.ErrInvalidVector) {
			t.Errorf("%s: err = %v, want ErrInvalidVector", name, err)
		}
	}
	if _, err := decodeBinary(kindSparsevec, build(vector.MaxSparseDim, 1, []uint32{vector.MaxSparseDim - 1})); err != nil {
		t.Errorf("dim 1e9 rejected: %v", err)
	}
}

func TestConvertVectorArgsPointers(t *testing.T) {
	v := vector.Vector{1, 2}
	h := vector.HalfVector{0.5}
	s := vector.SparseVector{Dim: 3, Indices: []int32{1}, Values: []float32{4}}
	b := vector.Bits{Len: 3, Data: []byte{0xA0}}
	var nilV *vector.Vector
	args := []any{&v, &h, &s, &b, nilV, 7, v}
	got := convertVectorArgs(VectorText, args)
	want := []any{"[1,2]", "[0.5]", "{2:4}/3", "101", nil, 7, "[1,2]"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %#v, want %#v", i, got[i], want[i])
		}
	}
	if _, ok := args[0].(*vector.Vector); !ok {
		t.Error("input slice was modified")
	}
	if out := convertVectorArgs(VectorBinary, args); &out[0] != &args[0] {
		t.Error("VectorBinary must leave arguments alone")
	}
}

func TestConvertVectorStmts(t *testing.T) {
	stmts := []momo.Compiled{
		{SQL: "a", Args: []any{1}},
		{SQL: "b", Args: []any{vector.Vector{1}}},
		{SQL: "c"},
	}
	out := convertVectorStmts(VectorText, stmts)
	if out[1].Args[0] != "[1]" {
		t.Fatalf("batch arg = %#v, want text", out[1].Args[0])
	}
	if _, ok := stmts[1].Args[0].(vector.Vector); !ok {
		t.Fatal("input statements were modified")
	}
	if got := convertVectorStmts(VectorBinary, stmts); &got[0] != &stmts[0] {
		t.Fatal("VectorBinary must not copy")
	}
}
