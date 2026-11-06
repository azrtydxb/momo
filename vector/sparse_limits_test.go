// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func sparseWithNNZ(dim, nnz int) SparseVector {
	s := SparseVector{Dim: dim, Indices: make([]int32, nnz), Values: make([]float32, nnz)}
	for i := range s.Indices {
		s.Indices[i] = int32(i)
		s.Values[i] = 1
	}
	return s
}

// TestSparseLimits pins the limits of the pgvector server: 1 <= dim <= 1e9 and
// at most 16000 stored elements (checked against pgvector with psql).
func TestSparseLimits(t *testing.T) {
	ok := []SparseVector{
		{Dim: 1},
		{Dim: MaxSparseDim},
		{Dim: MaxSparseDim, Indices: []int32{MaxSparseDim - 1}, Values: []float32{1}},
		sparseWithNNZ(MaxSparseNNZ, MaxSparseNNZ),
		sparseWithNNZ(1_000_000, MaxSparseNNZ),
	}
	for i, s := range ok {
		if err := s.Validate(s.Dim); err != nil {
			t.Errorf("case %d: Validate = %v, want nil", i, err)
		}
		if _, err := s.Value(); err != nil {
			t.Errorf("case %d: Value = %v, want nil", i, err)
		}
	}
	bad := map[string]SparseVector{
		"dim 0":           {Dim: 0},
		"dim over 1e9":    {Dim: MaxSparseDim + 1},
		"nnz 16001":       sparseWithNNZ(1_000_000, MaxSparseNNZ+1),
		"index == dim":    {Dim: MaxSparseDim, Indices: []int32{MaxSparseDim}, Values: []float32{1}},
		"index negative":  {Dim: 10, Indices: []int32{-1}, Values: []float32{1}},
		"not increasing":  {Dim: 10, Indices: []int32{3, 3}, Values: []float32{1, 1}},
		"values mismatch": {Dim: 10, Indices: []int32{3}},
	}
	for name, s := range bad {
		if err := s.Validate(s.Dim); !errors.Is(err, ErrInvalidVector) {
			t.Errorf("%s: Validate = %v, want ErrInvalidVector", name, err)
		}
		if _, err := s.Value(); !errors.Is(err, ErrInvalidVector) {
			t.Errorf("%s: Value = %v, want ErrInvalidVector", name, err)
		}
	}
}

func TestSparseScanLimits(t *testing.T) {
	var s SparseVector
	if err := s.Scan("{1:1}/1000000000"); err != nil || s.Dim != MaxSparseDim {
		t.Fatalf("Scan dim 1e9 = %+v, %v", s, err)
	}
	entries := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf("%d:1", i+1)
		}
		return "{" + strings.Join(parts, ",") + "}/1000000"
	}
	if err := s.Scan(entries(MaxSparseNNZ)); err != nil || len(s.Indices) != MaxSparseNNZ {
		t.Fatalf("Scan nnz 16000 = %v", err)
	}
	for name, text := range map[string]string{
		"dim over 1e9": "{1:1}/1000000001",
		"dim 0":        "{}/0",
		"nnz 16001":    entries(MaxSparseNNZ + 1),
		"index >= dim": "{6:1}/5",
		"decreasing":   "{2:1,1:1}/5",
	} {
		if err := s.Scan(text); !errors.Is(err, ErrInvalidVector) {
			t.Errorf("%s: Scan = %v, want ErrInvalidVector", name, err)
		}
	}
}
