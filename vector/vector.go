// SPDX-License-Identifier: Apache-2.0

// Package vector holds the vector value types.
package vector

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MaxDim is the storage limit for the dimension of vector and halfvec
// (pgvector README).
const MaxDim = 16000

// MaxSparseDim is the largest sparsevec dimension: pgvector accepts up to
// 1,000,000,000 dimensions, of which at most MaxSparseNNZ are non-zero.
const MaxSparseDim = 1_000_000_000

// MaxSparseNNZ is the largest number of stored elements in a sparsevec.
const MaxSparseNNZ = 16000

// MaxBits is PostgreSQL's limit for the length of a bit string.
const MaxBits = 83886080

// halfOverflow is the smallest magnitude that rounds to infinity in float16
// (the midpoint between 65504 and 65536).
const halfOverflow = 65520

// Vector is a pgvector vector (float32 elements).
type Vector []float32

// HalfVector is a pgvector halfvec; elements must fit in float16.
type HalfVector []float32

// SparseVector is a pgvector sparsevec. Indices are 0-based, strictly
// increasing, and each lies in [0, Dim).
type SparseVector struct {
	Dim     int
	Indices []int32
	Values  []float32
}

// Bits is a pgvector bit string of Len bits, packed most significant bit
// first into ceil(Len/8) bytes. Unused trailing bits must be zero.
type Bits struct {
	Len  int
	Data []byte
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidVector}, args...)...)
}

func checkDim(n, dim int) error {
	if n != dim {
		return invalid("dimension %d, want %d", n, dim)
	}
	if dim < 1 || dim > MaxDim {
		return invalid("dimension %d outside 1..%d", dim, MaxDim)
	}
	return nil
}

func finite(x float32) bool {
	f := float64(x)
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// Validate reports whether v has exactly dim finite elements and dim is in
// 1..MaxDim.
func (v Vector) Validate(dim int) error {
	if err := checkDim(len(v), dim); err != nil {
		return err
	}
	for i, x := range v {
		if !finite(x) {
			return invalid("element %d is not finite", i)
		}
	}
	return nil
}

// Validate is Vector.Validate plus the float16 range. Like pgvector, which
// converts to float16 with round-to-nearest-even, it accepts |x| < 65520
// (values in (65504, 65520) round down to 65504) and rejects larger ones.
func (h HalfVector) Validate(dim int) error {
	if err := checkDim(len(h), dim); err != nil {
		return err
	}
	for i, x := range h {
		if !finite(x) {
			return invalid("element %d is not finite", i)
		}
		if math.Abs(float64(x)) >= halfOverflow {
			return invalid("element %d (%v) exceeds the halfvec range", i, x)
		}
	}
	return nil
}

// Validate reports whether s has dimension dim and well-formed entries:
// 1 <= dim <= MaxSparseDim, at most MaxSparseNNZ entries, indices strictly
// increasing and inside [0, dim).
func (s SparseVector) Validate(dim int) error {
	if s.Dim != dim {
		return invalid("dimension %d, want %d", s.Dim, dim)
	}
	if dim < 1 || dim > MaxSparseDim {
		return invalid("dimension %d outside 1..%d", dim, MaxSparseDim)
	}
	if len(s.Indices) > MaxSparseNNZ {
		return invalid("%d non-zero elements exceed the limit of %d", len(s.Indices), MaxSparseNNZ)
	}
	if len(s.Indices) != len(s.Values) {
		return invalid("%d indices but %d values", len(s.Indices), len(s.Values))
	}
	for i, idx := range s.Indices {
		if idx < 0 || int(idx) >= s.Dim {
			return invalid("index %d outside [0, %d)", idx, s.Dim)
		}
		if i > 0 && idx <= s.Indices[i-1] {
			return invalid("indices must be strictly increasing: %d after %d", idx, s.Indices[i-1])
		}
		if !finite(s.Values[i]) {
			return invalid("value %d is not finite", i)
		}
	}
	return nil
}

// Validate reports whether b is a well-formed bit string of length dim.
func (b Bits) Validate(dim int) error {
	if b.Len != dim {
		return invalid("length %d, want %d", b.Len, dim)
	}
	if dim < 1 || dim > MaxBits {
		return invalid("length %d outside 1..%d", dim, MaxBits)
	}
	if want := (b.Len + 7) / 8; len(b.Data) != want {
		return invalid("%d data bytes for %d bits, want %d", len(b.Data), b.Len, want)
	}
	if r := b.Len % 8; r != 0 {
		if b.Data[len(b.Data)-1]&(byte(0xFF)>>r) != 0 {
			return invalid("unused trailing bits are not zero")
		}
	}
	return nil
}

// Value returns the pgvector text form, for example "[1,2.5,-3]".
func (v Vector) Value() (driver.Value, error) {
	if err := v.Validate(len(v)); err != nil {
		return nil, err
	}
	return formatDense(v), nil
}

// Value returns the pgvector text form, for example "[1,2]".
func (h HalfVector) Value() (driver.Value, error) {
	if err := h.Validate(len(h)); err != nil {
		return nil, err
	}
	return formatDense(h), nil
}

// Value returns the pgvector sparsevec text form, "{i:v,...}/dim" with
// 1-based indices.
func (s SparseVector) Value() (driver.Value, error) {
	if err := s.Validate(s.Dim); err != nil {
		return nil, err
	}
	var sb strings.Builder
	sb.WriteByte('{')
	for i, idx := range s.Indices {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatInt(int64(idx)+1, 10))
		sb.WriteByte(':')
		sb.WriteString(formatFloat(s.Values[i]))
	}
	sb.WriteString("}/")
	sb.WriteString(strconv.Itoa(s.Dim))
	return sb.String(), nil
}

// Value returns the bit string as a string of 0 and 1.
func (b Bits) Value() (driver.Value, error) {
	if err := b.Validate(b.Len); err != nil {
		return nil, err
	}
	out := make([]byte, b.Len)
	for i := range out {
		out[i] = '0' + (b.Data[i/8]>>(7-uint(i%8)))&1
	}
	return string(out), nil
}

func formatFloat(x float32) string {
	return strconv.FormatFloat(float64(x), 'g', -1, 32)
}

func formatDense(v []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(formatFloat(x))
	}
	sb.WriteByte(']')
	return sb.String()
}

func parseFloat(s string) (float32, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
	if err != nil {
		return 0, invalid("bad number %q", s)
	}
	if !finite(float32(f)) {
		return 0, invalid("number %q is not finite", s)
	}
	return float32(f), nil
}

func parseDense(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, invalid("text %q is not of the form [a,b,...]", s)
	}
	body := strings.TrimSpace(s[1 : len(s)-1])
	if body == "" {
		return nil, invalid("empty vector")
	}
	parts := strings.Split(body, ",")
	if len(parts) > MaxDim {
		return nil, invalid("dimension %d outside 1..%d", len(parts), MaxDim)
	}
	out := make([]float32, len(parts))
	for i, p := range parts {
		x, err := parseFloat(p)
		if err != nil {
			return nil, err
		}
		out[i] = x
	}
	return out, nil
}

func decodeFloat32LE(b []byte) ([]float32, error) {
	if len(b)%4 != 0 || len(b) == 0 {
		return nil, invalid("blob of %d bytes is not a float32 array", len(b))
	}
	if len(b)/4 > MaxDim {
		return nil, invalid("dimension %d outside 1..%d", len(b)/4, MaxDim)
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out, nil
}

// scanDense implements Scan for the two dense types: a string is pgvector
// text, a []byte is a sqlite-vec little-endian float32 blob.
func scanDense(src any, check func([]float32) error) ([]float32, error) {
	var out []float32
	var err error
	switch s := src.(type) {
	case nil:
		return nil, nil
	case string:
		out, err = parseDense(s)
	case []byte:
		out, err = decodeFloat32LE(s)
	default:
		return nil, invalid("cannot scan %T", src)
	}
	if err != nil {
		return nil, err
	}
	if err := check(out); err != nil {
		return nil, err
	}
	return out, nil
}

// Scan accepts pgvector text (string) or a sqlite-vec blob ([]byte).
func (v *Vector) Scan(src any) error {
	out, err := scanDense(src, func(f []float32) error { return Vector(f).Validate(len(f)) })
	if err != nil {
		return err
	}
	*v = out
	return nil
}

// Scan accepts pgvector text (string) or a []byte float32 blob.
func (h *HalfVector) Scan(src any) error {
	out, err := scanDense(src, func(f []float32) error { return HalfVector(f).Validate(len(f)) })
	if err != nil {
		return err
	}
	*h = out
	return nil
}

// Scan accepts pgvector sparsevec text ("{2:1,4:2}/5").
func (s *SparseVector) Scan(src any) error {
	switch t := src.(type) {
	case nil:
		*s = SparseVector{}
		return nil
	case string:
		out, err := parseSparse(t)
		if err != nil {
			return err
		}
		*s = out
		return nil
	}
	return invalid("cannot scan %T", src)
}

func parseSparse(text string) (SparseVector, error) {
	text = strings.TrimSpace(text)
	body, dimText, ok := strings.Cut(text, "/")
	if !ok || len(body) < 2 || body[0] != '{' || body[len(body)-1] != '}' {
		return SparseVector{}, invalid("text %q is not of the form {i:v,...}/dim", text)
	}
	dim, err := strconv.Atoi(strings.TrimSpace(dimText))
	if err != nil {
		return SparseVector{}, invalid("bad dimension %q", dimText)
	}
	out := SparseVector{Dim: dim}
	if inner := strings.TrimSpace(body[1 : len(body)-1]); inner != "" {
		entries := strings.Split(inner, ",")
		if len(entries) > MaxSparseNNZ {
			return SparseVector{}, invalid("%d non-zero elements exceed the limit of %d", len(entries), MaxSparseNNZ)
		}
		for _, entry := range entries {
			idxText, valText, ok := strings.Cut(entry, ":")
			if !ok {
				return SparseVector{}, invalid("bad entry %q", entry)
			}
			idx, err := strconv.ParseInt(strings.TrimSpace(idxText), 10, 32)
			if err != nil || idx < 1 {
				return SparseVector{}, invalid("bad index %q (1-based)", idxText)
			}
			val, err := parseFloat(valText)
			if err != nil {
				return SparseVector{}, err
			}
			out.Indices = append(out.Indices, int32(idx-1))
			out.Values = append(out.Values, val)
		}
	}
	if err := out.Validate(dim); err != nil {
		return SparseVector{}, err
	}
	return out, nil
}

// Scan accepts a string of 0 and 1, or a packed []byte blob (sqlite-vec),
// whose Len becomes 8 times its length.
func (b *Bits) Scan(src any) error {
	switch t := src.(type) {
	case nil:
		*b = Bits{}
		return nil
	case string:
		t = strings.TrimSpace(t)
		if len(t) < 1 || len(t) > MaxBits {
			return invalid("bit string length %d outside 1..%d", len(t), MaxBits)
		}
		data := make([]byte, (len(t)+7)/8)
		for i := 0; i < len(t); i++ {
			switch t[i] {
			case '0':
			case '1':
				data[i/8] |= 0x80 >> uint(i%8)
			default:
				return invalid("bit string has character %q", t[i])
			}
		}
		*b = Bits{Len: len(t), Data: data}
		return nil
	case []byte:
		if len(t) == 0 || len(t) > MaxBits/8 {
			return invalid("bit blob of %d bytes", len(t))
		}
		*b = Bits{Len: 8 * len(t), Data: append([]byte(nil), t...)}
		return nil
	}
	return invalid("cannot scan %T", src)
}
