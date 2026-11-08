// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"errors"
	"math"
	"reflect"
	"runtime"
	"testing"

	"github.com/azrtydxb/momo/vector"
)

func TestVectorBinaryDecodeRejectsMalformed(t *testing.T) {
	bad := map[vectorKind][][]byte{
		kindVector:    {nil, {0, 1}, {0, 1, 0, 0}, {0, 1, 0, 1, 0, 0, 0, 0}, {0, 1, 0, 0, 0x7f, 0xc0, 0, 0}},
		kindHalfvec:   {nil, {0, 1, 0, 0}, {0, 2, 0, 0, 0, 0}},
		kindSparsevec: {nil, make([]byte, 11), {0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0, 0}, {0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 9, 0x3f, 0x80, 0, 0}},
		kindBit:       {nil, {0, 0, 0}, {0, 0, 0, 9, 0xff}, {0, 0, 0, 9, 0xff, 0xff}},
	}
	for kind, inputs := range bad {
		for _, b := range inputs {
			if _, err := decodeBinary(kind, b); err == nil {
				t.Errorf("kind %d: % x accepted", kind, b)
			}
		}
	}
}

func FuzzVectorBinaryDecode(f *testing.F) {
	f.Add(uint8(0), []byte{0, 3, 0, 0, 0x3f, 0x80, 0, 0, 0x40, 0x20, 0, 0, 0xc0, 0x40, 0, 0})
	f.Add(uint8(1), []byte{0, 1, 0, 0, 0x38, 0})
	f.Add(uint8(2), []byte{0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 4, 0x3f, 0x80, 0, 0})
	f.Add(uint8(3), []byte{0, 0, 0, 9, 0xaa, 0x80})
	f.Fuzz(func(t *testing.T, k uint8, data []byte) {
		kind := vectorKind(k % 4)
		v, err := decodeBinary(kind, data)
		if err != nil {
			return
		}
		out, err := encodeBinary(kind, v, nil)
		if err != nil {
			t.Fatalf("decoded %+v does not re-encode: %v", v, err)
		}
		again, err := decodeBinary(kind, out)
		if err != nil || !reflect.DeepEqual(again, v) {
			t.Fatalf("second decode %+v err %v, want %+v", again, err, v)
		}
	})
}

// allocated reports the bytes decodeBinary allocates for b.
func allocated(kind vectorKind, b []byte) (n uint64, err error) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err = decodeBinary(kind, b)
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc, err
}

// TestVectorBinaryDecodeRejectsOversizedBeforeAllocating feeds headers whose
// declared sizes are consistent with the payload length but beyond pgvector's
// limits (or negative once read as int32): the decode must fail on the bound,
// not after allocating for it.
func TestVectorBinaryDecodeRejectsOversizedBeforeAllocating(t *testing.T) {
	const budget = 32 << 10
	cases := []struct {
		name string
		kind vectorKind
		b    []byte
	}{
		{"vector dim 0xFFFF", kindVector, append([]byte{0xff, 0xff, 0, 0}, make([]byte, 4*0xffff)...)},
		{"halfvec dim 0xFFFF", kindHalfvec, append([]byte{0xff, 0xff, 0, 0}, make([]byte, 2*0xffff)...)},
		{"vector dim 16001", kindVector, append([]byte{0x3e, 0x81, 0, 0}, make([]byte, 4*16001)...)},
		{"sparsevec nnz 16001", kindSparsevec, append([]byte{0, 0, 0x27, 0x10, 0, 0, 0x3e, 0x81, 0, 0, 0, 0}, make([]byte, 8*16001)...)},
		{"sparsevec negative nnz", kindSparsevec, []byte{0, 0, 0, 5, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0}},
		{"sparsevec negative dim", kindSparsevec, []byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"sparsevec dim 0xFFFFFFFE", kindSparsevec, []byte{0xff, 0xff, 0xff, 0xfe, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"sparsevec dim over max", kindSparsevec, []byte{0x3b, 0x9a, 0xca, 0x01, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"bit negative length", kindBit, []byte{0x80, 0, 0, 0}},
		{"bit over max", kindBit, []byte{0x05, 0, 0, 1}},
	}
	for _, tc := range cases {
		n, err := allocated(tc.kind, tc.b)
		if err == nil || !errors.Is(err, vector.ErrInvalidVector) {
			t.Errorf("%s: err = %v, want ErrInvalidVector", tc.name, err)
		}
		if n > budget {
			t.Errorf("%s: decode allocated %d bytes before rejecting, budget %d", tc.name, n, budget)
		}
	}
}

func TestSparseIndexOutsideDimRejected(t *testing.T) {
	// dim 5, one entry whose index 0x80000000 is negative as int32.
	b := []byte{0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0, 0, 0x80, 0, 0, 0, 0x3f, 0x80, 0, 0}
	if _, err := decodeBinary(kindSparsevec, b); !errors.Is(err, vector.ErrInvalidVector) {
		t.Fatalf("err = %v, want ErrInvalidVector", err)
	}
}

func TestOIDFromInt64(t *testing.T) {
	for _, v := range []int64{0, 1, 16384, math.MaxUint32} {
		oid, err := oidFromInt64(v)
		if err != nil || int64(oid) != v {
			t.Errorf("oidFromInt64(%d) = %d, %v", v, oid, err)
		}
	}
	for _, v := range []int64{-1, math.MaxUint32 + 1, math.MinInt64, math.MaxInt64} {
		if _, err := oidFromInt64(v); err == nil {
			t.Errorf("oidFromInt64(%d) accepted", v)
		}
	}
}
