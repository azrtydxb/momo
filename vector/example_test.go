// SPDX-License-Identifier: Apache-2.0

package vector_test

import (
	"errors"
	"fmt"
	"math"

	"github.com/azrtydxb/momo/vector"
)

// A Vector is checked against the declared column dimension, and its Value
// is the pgvector text form.
func ExampleVector() {
	v := vector.Vector{1, 2.5, -3}
	fmt.Println(v.Validate(3))
	text, _ := v.Value()
	fmt.Println(text)

	err := v.Validate(4)
	fmt.Println(errors.Is(err, vector.ErrInvalidVector))
	// Output:
	// <nil>
	// [1,2.5,-3]
	// true
}

// NaN and infinite components are rejected, and so is a dimension above
// MaxDim.
func ExampleVector_Validate() {
	nan := float32(math.NaN())
	fmt.Println(errors.Is(vector.Vector{1, nan}.Validate(2), vector.ErrInvalidVector))
	fmt.Println(vector.MaxDim)
	// Output:
	// true
	// 16000
}

// A HalfVector holds float16 values, so each component must fit in 65504.
func ExampleHalfVector() {
	fmt.Println(vector.HalfVector{1, 65504}.Validate(2))
	fmt.Println(errors.Is(vector.HalfVector{1, 70000}.Validate(2), vector.ErrInvalidVector))
	// Output:
	// <nil>
	// true
}

// A SparseVector lists zero-based, strictly increasing indices. Its text
// form is pgvector's {index:value,...}/dimension, with one-based indices.
func ExampleSparseVector() {
	s := vector.SparseVector{Dim: 5, Indices: []int32{0, 2}, Values: []float32{1.5, 2}}
	fmt.Println(s.Validate(5))
	text, _ := s.Value()
	fmt.Println(text)

	unsorted := vector.SparseVector{Dim: 5, Indices: []int32{2, 0}, Values: []float32{1, 1}}
	fmt.Println(errors.Is(unsorted.Validate(5), vector.ErrInvalidVector))
	// Output:
	// <nil>
	// {1:1.5,3:2}/5
	// true
}

// Bits is a bit string packed most significant bit first.
func ExampleBits() {
	b := vector.Bits{Len: 4, Data: []byte{0b0101_0000}}
	fmt.Println(b.Validate(4))
	text, _ := b.Value()
	fmt.Println(text)

	// A set bit beyond Len is rejected.
	dirty := vector.Bits{Len: 4, Data: []byte{0b0101_0001}}
	fmt.Println(errors.Is(dirty.Validate(4), vector.ErrInvalidVector))
	// Output:
	// <nil>
	// 0101
	// true
}

// Scan reads the pgvector text form back.
func ExampleVector_Scan() {
	var v vector.Vector
	if err := v.Scan("[1,2.5,-3]"); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v)
	// Output:
	// [1 2.5 -3]
}
