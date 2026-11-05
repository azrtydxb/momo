// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"errors"
	"math"
	"testing"
)

func TestVectorCodec(t *testing.T) {
	v := Vector{1, 2.5, -3}
	val, err := v.Value()
	if err != nil {
		t.Fatal(err)
	}
	if val != "[1,2.5,-3]" {
		t.Fatalf("text encoding = %v, want [1,2.5,-3]", val)
	}
	var got Vector
	if err := got.Scan("[1,2.5,-3]"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2.5 || got[2] != -3 {
		t.Fatalf("round trip = %v", got)
	}
	if err := v.Validate(2); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("dimension 3 against 2: err = %v, want ErrInvalidVector", err)
	}
	if err := (Vector{float32(math.NaN())}).Validate(1); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("NaN: err = %v, want ErrInvalidVector", err)
	}
	if err := (Vector{float32(math.Inf(1))}).Validate(1); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("Inf: err = %v, want ErrInvalidVector", err)
	}
	if err := (HalfVector{70000}).Validate(1); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("halfvec out of range: err = %v, want ErrInvalidVector", err)
	}
	s := SparseVector{Dim: 5, Indices: []int32{1, 3}, Values: []float32{1, 2}}
	sv, err := s.Value()
	if err != nil {
		t.Fatal(err)
	}
	if sv != "{2:1,4:2}/5" {
		t.Fatalf("sparse text = %v, want {2:1,4:2}/5 (pgvector literals are 1-based)", sv)
	}
	if err := (SparseVector{Dim: 5, Indices: []int32{3, 1}, Values: []float32{1, 2}}).Validate(5); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("unsorted sparse indices: err = %v, want ErrInvalidVector", err)
	}
	b := Bits{Len: 4, Data: []byte{0xA0}}
	bv, err := b.Value()
	if err != nil {
		t.Fatal(err)
	}
	if bv != "1010" {
		t.Fatalf("bits text = %v, want 1010", bv)
	}
	if err := (Bits{Len: 4, Data: []byte{0xA1}}).Validate(4); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("nonzero trailing bits: err = %v, want ErrInvalidVector", err)
	}
}

func TestValidateRules(t *testing.T) {
	big := make([]float32, MaxDim+1)
	cases := []struct {
		name string
		err  error
	}{
		{"zero dim vector", Vector{}.Validate(0)},
		{"vector over max", Vector(big).Validate(MaxDim + 1)},
		{"halfvec over max", HalfVector(big).Validate(MaxDim + 1)},
		{"halfvec NaN", HalfVector{float32(math.NaN())}.Validate(1)},
		{"halfvec -Inf", HalfVector{float32(math.Inf(-1))}.Validate(1)},
		{"halfvec -70000", HalfVector{-70000}.Validate(1)},
		{"sparse zero dim", SparseVector{}.Validate(0)},
		{"sparse over max", SparseVector{Dim: MaxSparseDim + 1}.Validate(MaxSparseDim + 1)},
		{"sparse dim mismatch", SparseVector{Dim: 5}.Validate(4)},
		{"sparse index == dim", SparseVector{Dim: 5, Indices: []int32{5}, Values: []float32{1}}.Validate(5)},
		{"sparse negative index", SparseVector{Dim: 5, Indices: []int32{-1}, Values: []float32{1}}.Validate(5)},
		{"sparse duplicate", SparseVector{Dim: 5, Indices: []int32{2, 2}, Values: []float32{1, 1}}.Validate(5)},
		{"sparse len mismatch", SparseVector{Dim: 5, Indices: []int32{1}, Values: nil}.Validate(5)},
		{"sparse NaN", SparseVector{Dim: 5, Indices: []int32{1}, Values: []float32{float32(math.NaN())}}.Validate(5)},
		{"bits zero", Bits{}.Validate(0)},
		{"bits len mismatch", Bits{Len: 4, Data: []byte{0xA0}}.Validate(8)},
		{"bits data too short", Bits{Len: 9, Data: []byte{0xFF}}.Validate(9)},
	}
	for _, c := range cases {
		if !errors.Is(c.err, ErrInvalidVector) {
			t.Errorf("%s: err = %v, want ErrInvalidVector", c.name, c.err)
		}
	}
	ok := []error{
		Vector{1}.Validate(1),
		Vector(make([]float32, MaxDim)).Validate(MaxDim),
		HalfVector{65504, -65504}.Validate(2),
		SparseVector{Dim: 5, Indices: []int32{0, 4}, Values: []float32{1, 2}}.Validate(5),
		Bits{Len: 9, Data: []byte{0xFF, 0x80}}.Validate(9),
	}
	for i, err := range ok {
		if err != nil {
			t.Errorf("valid case %d: %v", i, err)
		}
	}
}

func TestRoundTrips(t *testing.T) {
	var h HalfVector
	if err := h.Scan("[1,-2.5]"); err != nil || len(h) != 2 || h[1] != -2.5 {
		t.Fatalf("halfvec scan = %v, %v", h, err)
	}
	var s SparseVector
	if err := s.Scan("{2:1,4:2}/5"); err != nil {
		t.Fatal(err)
	}
	if s.Dim != 5 || len(s.Indices) != 2 || s.Indices[0] != 1 || s.Indices[1] != 3 || s.Values[1] != 2 {
		t.Fatalf("sparse scan = %+v", s)
	}
	enc, err := s.Value()
	if err != nil || enc != "{2:1,4:2}/5" {
		t.Fatalf("sparse re-encode = %v, %v", enc, err)
	}
	var b Bits
	if err := b.Scan("1010"); err != nil || b.Len != 4 || len(b.Data) != 1 || b.Data[0] != 0xA0 {
		t.Fatalf("bits scan = %+v, %v", b, err)
	}
	var blob Bits
	if err := blob.Scan([]byte{0xAB, 0xCD}); err != nil || blob.Len != 16 {
		t.Fatalf("bits blob scan = %+v, %v", blob, err)
	}
	var v Vector
	if err := v.Scan([]byte{0, 0, 0x80, 0x3f, 0, 0, 0, 0x40}); err != nil || len(v) != 2 || v[0] != 1 || v[1] != 2 {
		t.Fatalf("blob scan = %v, %v", v, err)
	}
	if err := v.Scan([]byte{1, 2, 3}); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("short blob err = %v", err)
	}
	for _, bad := range []string{"", "[]", "[NaN]", "[Inf]", "1,2", "[1,,2]", "[a]"} {
		if err := v.Scan(bad); !errors.Is(err, ErrInvalidVector) {
			t.Errorf("Scan(%q) err = %v, want ErrInvalidVector", bad, err)
		}
	}
	for _, bad := range []string{"{2:1}", "{0:1}/5", "{6:1}/5", "{2:1,2:3}/5", "{a:1}/5", "{2:NaN}/5", "{2:1}/0"} {
		if err := s.Scan(bad); !errors.Is(err, ErrInvalidVector) {
			t.Errorf("sparse Scan(%q) err = %v, want ErrInvalidVector", bad, err)
		}
	}
	if err := b.Scan("10x1"); !errors.Is(err, ErrInvalidVector) {
		t.Errorf("bits bad text err = %v", err)
	}
	if err := v.Scan(42); !errors.Is(err, ErrInvalidVector) {
		t.Errorf("Scan(int) err = %v", err)
	}
}
