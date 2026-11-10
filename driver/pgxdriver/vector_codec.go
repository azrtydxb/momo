// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	sqldriver "database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/azrtydxb/momo/types"
	"github.com/azrtydxb/momo/vector"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VectorFormat selects how the driver sends vector values on the wire.
type VectorFormat int

const (
	// VectorText is the default: text encoding for every vector type.
	VectorText VectorFormat = iota
	// VectorBinary sends pgvector's binary format. It requires
	// ConfigureVectors on the pool, and a describe-based exec mode
	// (pgx.QueryExecModeDescribeExec, CacheStatement or CacheDescribe):
	// pgx's QueryExecModeExec, the default, always sends text parameters and
	// asks for text results, so there the text encoding is used and the
	// result is identical.
	VectorBinary
)

// WithVectorFormat sets the vector wire format. VectorText converts vector
// arguments to their text form before pgx sees them, so a pool configured with
// ConfigureVectors still sends text, for Exec, Query, transactions, pinned
// sessions and batches alike. COPY always uses binary and so needs
// ConfigureVectors. With VectorBinary, batches sent in a non-Exec mode use
// binary whenever ConfigureVectors is installed.
func WithVectorFormat(f VectorFormat) Option {
	return func(d *driver) { d.vecFormat = f }
}

// vectorKind identifies one of the four pgvector wire types.
type vectorKind uint8

const (
	kindVector vectorKind = iota
	kindHalfvec
	kindSparsevec
	kindBit
)

// vectorTypes lists the registered types. vector, halfvec and sparsevec come
// from the pgvector extension; bit is a PostgreSQL built-in whose codec is
// extended, not replaced.
var vectorTypes = []struct {
	name string
	kind vectorKind
	goT  any
}{
	{"vector", kindVector, vector.Vector{}},
	{"halfvec", kindHalfvec, vector.HalfVector{}},
	{"sparsevec", kindSparsevec, vector.SparseVector{}},
	{"bit", kindBit, vector.Bits{}},
}

// ConfigureVectors installs an AfterConnect hook on cfg that looks up the OIDs
// of vector, halfvec, sparsevec and bit and registers pgtype codecs for them.
// It must be called before pgxpool.NewWithConfig. The previous AfterConnect
// hook runs first. Extension type OIDs are per database, so they are looked up
// on every new connection (through pg_extension, independent of search_path);
// when the extension is not installed its types are skipped and no error is
// raised: vector values then travel as text and the server reports the missing
// type. The lookup runs once per connection, when it is opened: a connection
// opened before CREATE EXTENSION vector stays on text until the pool recycles
// it (MaxConnLifetime, MaxConnIdleTime, or pool.Reset).
func ConfigureVectors(cfg *pgxpool.Config) {
	prev := cfg.AfterConnect
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		if prev != nil {
			if err := prev(ctx, c); err != nil {
				return err
			}
		}
		return registerVectorTypes(ctx, c)
	}
}

func registerVectorTypes(ctx context.Context, c *pgx.Conn) error {
	tm := c.TypeMap()
	for _, vt := range vectorTypes {
		var oid uint32
		if vt.kind == kindBit {
			oid = pgtype.BitOID
		} else {
			var found bool
			var err error
			oid, found, err = lookupExtensionType(ctx, c, "vector", vt.name)
			if err != nil {
				return fmt.Errorf("pgxdriver: look up type %s: %w", vt.name, err)
			}
			if !found {
				continue
			}
		}
		codec := &vectorCodec{kind: vt.kind}
		if prev, ok := tm.TypeForOID(oid); ok {
			codec.fallback = prev.Codec
		}
		tm.RegisterType(&pgtype.Type{Name: vt.name, OID: oid, Codec: codec})
		tm.RegisterDefaultPgType(vt.goT, vt.name)
	}
	return nil
}

// lookupExtensionType returns the OID of type typname owned by extension ext.
// found is false, with a nil error, when the extension or type is absent.
func lookupExtensionType(ctx context.Context, c *pgx.Conn, ext, typname string) (oid uint32, found bool, err error) {
	const q = `SELECT t.oid::int8 FROM pg_type t
JOIN pg_depend d ON d.classid = 'pg_type'::regclass AND d.objid = t.oid AND d.deptype = 'e'
JOIN pg_extension e ON e.oid = d.refobjid
WHERE e.extname = $1 AND t.typname = $2`
	var v int64
	err = c.QueryRow(ctx, q, pgx.QueryExecModeExec, ext, typname).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	oid, err = oidFromInt64(v)
	if err != nil {
		return 0, false, err
	}
	return oid, true, nil
}

// oidFromInt64 narrows an oid read as int8 (0..2^32-1 on the server) to uint32.
func oidFromInt64(v int64) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, fmt.Errorf("pgxdriver: type oid %d outside uint32 range", v)
	}
	return uint32(v), nil
}

// convertVectorArgs makes VectorText independent of the pool's codecs: vector
// values, and pointers to them, become their text form (nil pointers become
// NULL). Values that fail validation are left as they
// are so the error surfaces from the usual path.
func convertVectorArgs(f VectorFormat, args []any) []any {
	args = convertUUIDArrayArgs(args)
	if f != VectorText {
		return args
	}
	var out []any
	for i, a := range args {
		v, isNull, ok := vectorValuer(a)
		if !ok {
			continue
		}
		if isNull {
			if out == nil {
				out = append([]any(nil), args...)
			}
			out[i] = nil
			continue
		}
		s, err := v.Value()
		if err != nil {
			continue
		}
		if out == nil {
			out = append([]any(nil), args...)
		}
		out[i] = s
	}
	if out == nil {
		return args
	}
	return out
}

// vectorValuer recognises a vector.* value or a pointer to one. isNull
// reports a nil pointer, which is sent as NULL.
func vectorValuer(a any) (v sqldriver.Valuer, isNull, ok bool) {
	switch t := a.(type) {
	case vector.Vector:
		return t, false, true
	case vector.HalfVector:
		return t, false, true
	case vector.SparseVector:
		return t, false, true
	case vector.Bits:
		return t, false, true
	case *vector.Vector:
		if t == nil {
			return nil, true, true
		}
		return *t, false, true
	case *vector.HalfVector:
		if t == nil {
			return nil, true, true
		}
		return *t, false, true
	case *vector.SparseVector:
		if t == nil {
			return nil, true, true
		}
		return *t, false, true
	case *vector.Bits:
		if t == nil {
			return nil, true, true
		}
		return *t, false, true
	}
	return nil, false, false
}

// vectorCodec implements pgtype.Codec for one pgvector type, in text and
// binary format. fallback, when set, handles every value that is not a
// vector.* type (the built-in bit codec).
type vectorCodec struct {
	kind     vectorKind
	fallback pgtype.Codec
}

func (c *vectorCodec) FormatSupported(f int16) bool {
	return f == pgtype.TextFormatCode || f == pgtype.BinaryFormatCode
}

func (c *vectorCodec) PreferredFormat() int16 { return pgtype.BinaryFormatCode }

// unwrap returns the value behind a vector.* value or non-nil pointer to one.
// isNull reports a nil pointer; ok is false for any other type or kind.
func (c *vectorCodec) unwrap(value any) (v any, isNull, ok bool) {
	switch t := value.(type) {
	case vector.Vector:
		return t, false, c.kind == kindVector
	case *vector.Vector:
		return derefOrNull(t), t == nil, c.kind == kindVector
	case vector.HalfVector:
		return t, false, c.kind == kindHalfvec
	case *vector.HalfVector:
		return derefOrNull(t), t == nil, c.kind == kindHalfvec
	case vector.SparseVector:
		return t, false, c.kind == kindSparsevec
	case *vector.SparseVector:
		return derefOrNull(t), t == nil, c.kind == kindSparsevec
	case vector.Bits:
		return t, false, c.kind == kindBit
	case *vector.Bits:
		return derefOrNull(t), t == nil, c.kind == kindBit
	}
	return nil, false, false
}

func derefOrNull[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func (c *vectorCodec) PlanEncode(m *pgtype.Map, oid uint32, format int16, value any) pgtype.EncodePlan {
	if _, _, ok := c.unwrap(value); !ok {
		if c.fallback != nil {
			return c.fallback.PlanEncode(m, oid, format, value)
		}
		return nil
	}
	switch format {
	case pgtype.BinaryFormatCode:
		return vectorEncodePlan{c: c, binary: true}
	case pgtype.TextFormatCode:
		return vectorEncodePlan{c: c}
	}
	return nil
}

type vectorEncodePlan struct {
	c      *vectorCodec
	binary bool
}

func (p vectorEncodePlan) Encode(value any, buf []byte) ([]byte, error) {
	v, isNull, _ := p.c.unwrap(value)
	if isNull {
		return nil, nil
	}
	if p.binary {
		return encodeBinary(p.c.kind, v, buf)
	}
	s, err := v.(sqldriver.Valuer).Value()
	if err != nil {
		return nil, err
	}
	return append(buf, s.(string)...), nil
}

func (c *vectorCodec) PlanScan(m *pgtype.Map, oid uint32, format int16, target any) pgtype.ScanPlan {
	if !c.accepts(target) {
		if c.fallback != nil {
			return c.fallback.PlanScan(m, oid, format, target)
		}
		return nil
	}
	switch format {
	case pgtype.BinaryFormatCode:
		return vectorScanPlan{c: c, binary: true}
	case pgtype.TextFormatCode:
		return vectorScanPlan{c: c}
	}
	return nil
}

func (c *vectorCodec) accepts(target any) bool {
	switch target.(type) {
	case *vector.Vector:
		return c.kind == kindVector
	case *vector.HalfVector:
		return c.kind == kindHalfvec
	case *vector.SparseVector:
		return c.kind == kindSparsevec
	case *vector.Bits:
		return c.kind == kindBit
	}
	return false
}

type vectorScanPlan struct {
	c      *vectorCodec
	binary bool
}

func (p vectorScanPlan) Scan(src []byte, target any) error {
	if src == nil {
		return target.(interface{ Scan(any) error }).Scan(nil)
	}
	if !p.binary {
		return target.(interface{ Scan(any) error }).Scan(string(src))
	}
	v, err := decodeBinary(p.c.kind, src)
	if err != nil {
		return err
	}
	switch t := target.(type) {
	case *vector.Vector:
		*t = v.(vector.Vector)
	case *vector.HalfVector:
		*t = v.(vector.HalfVector)
	case *vector.SparseVector:
		*t = v.(vector.SparseVector)
	case *vector.Bits:
		*t = v.(vector.Bits)
	}
	return nil
}

func (c *vectorCodec) DecodeDatabaseSQLValue(m *pgtype.Map, oid uint32, format int16, src []byte) (sqldriver.Value, error) {
	if c.fallback != nil {
		return c.fallback.DecodeDatabaseSQLValue(m, oid, format, src)
	}
	if src == nil {
		return nil, nil
	}
	if format == pgtype.TextFormatCode {
		return string(src), nil
	}
	v, err := decodeBinary(c.kind, src)
	if err != nil {
		return nil, err
	}
	return v.(sqldriver.Valuer).Value()
}

func (c *vectorCodec) DecodeValue(m *pgtype.Map, oid uint32, format int16, src []byte) (any, error) {
	if c.fallback != nil {
		return c.fallback.DecodeValue(m, oid, format, src)
	}
	if src == nil {
		return nil, nil
	}
	if format == pgtype.BinaryFormatCode {
		return decodeBinary(c.kind, src)
	}
	switch c.kind {
	case kindVector:
		var v vector.Vector
		return v, v.Scan(string(src))
	case kindHalfvec:
		var v vector.HalfVector
		return v, v.Scan(string(src))
	default:
		var v vector.SparseVector
		return v, v.Scan(string(src))
	}
}

// encodeBinary appends pgvector's send format of v (a vector.* value of the
// given kind) to buf.
func encodeBinary(kind vectorKind, v any, buf []byte) ([]byte, error) {
	switch kind {
	case kindVector:
		x, ok := v.(vector.Vector)
		if !ok {
			return nil, fmt.Errorf("pgxdriver: %T is not a vector.Vector", v)
		}
		if err := x.Validate(len(x)); err != nil {
			return nil, err
		}
		buf = binary.BigEndian.AppendUint16(buf, uint16(len(x))) // #nosec G115 -- x.Validate(len(x)) above bounds the length to 1..vector.MaxDim (16000)
		buf = binary.BigEndian.AppendUint16(buf, 0)
		for _, f := range x {
			buf = binary.BigEndian.AppendUint32(buf, math.Float32bits(f))
		}
		return buf, nil
	case kindHalfvec:
		x, ok := v.(vector.HalfVector)
		if !ok {
			return nil, fmt.Errorf("pgxdriver: %T is not a vector.HalfVector", v)
		}
		if err := x.Validate(len(x)); err != nil {
			return nil, err
		}
		buf = binary.BigEndian.AppendUint16(buf, uint16(len(x))) // #nosec G115 -- x.Validate(len(x)) above bounds the length to 1..vector.MaxDim (16000)
		buf = binary.BigEndian.AppendUint16(buf, 0)
		for i, f := range x {
			h, ok := float32ToHalf(f)
			if !ok {
				return nil, fmt.Errorf("%w: element %d (%v) overflows halfvec", vector.ErrInvalidVector, i, f)
			}
			buf = binary.BigEndian.AppendUint16(buf, h)
		}
		return buf, nil
	case kindSparsevec:
		x, ok := v.(vector.SparseVector)
		if !ok {
			return nil, fmt.Errorf("pgxdriver: %T is not a vector.SparseVector", v)
		}
		if err := x.Validate(x.Dim); err != nil {
			return nil, err
		}
		buf = binary.BigEndian.AppendUint32(buf, uint32(x.Dim))          // #nosec G115 -- x.Validate(x.Dim) above bounds Dim to 1..vector.MaxSparseDim (1e9)
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(x.Indices))) // #nosec G115 -- x.Validate above bounds the entry count to vector.MaxSparseNNZ (16000)
		buf = binary.BigEndian.AppendUint32(buf, 0)
		for _, i := range x.Indices {
			buf = binary.BigEndian.AppendUint32(buf, uint32(i)) // #nosec G115 -- x.Validate above requires 0 <= index < Dim <= 1e9
		}
		for _, f := range x.Values {
			buf = binary.BigEndian.AppendUint32(buf, math.Float32bits(f))
		}
		return buf, nil
	case kindBit:
		x, ok := v.(vector.Bits)
		if !ok {
			return nil, fmt.Errorf("pgxdriver: %T is not a vector.Bits", v)
		}
		if err := x.Validate(x.Len); err != nil {
			return nil, err
		}
		buf = binary.BigEndian.AppendUint32(buf, uint32(x.Len)) // #nosec G115 -- x.Validate(x.Len) above bounds Len to 1..vector.MaxBits (83886080)
		return append(buf, x.Data...), nil
	}
	return nil, fmt.Errorf("pgxdriver: unknown vector kind %d", kind)
}

func badBinary(kind vectorKind, format string, args ...any) error {
	names := [...]string{"vector", "halfvec", "sparsevec", "bit"}
	return fmt.Errorf("%w: binary %s: %s", vector.ErrInvalidVector, names[kind], fmt.Sprintf(format, args...))
}

// decodeBinary parses pgvector's send format into the matching vector.* value.
// Every length is checked against len(b) before anything is allocated.
func decodeBinary(kind vectorKind, b []byte) (any, error) {
	switch kind {
	case kindVector, kindHalfvec:
		if len(b) < 4 {
			return nil, badBinary(kind, "%d bytes, need a 4-byte header", len(b))
		}
		dim := int(binary.BigEndian.Uint16(b))
		if dim < 1 || dim > vector.MaxDim {
			return nil, badBinary(kind, "dimension %d outside 1..%d", dim, vector.MaxDim)
		}
		if binary.BigEndian.Uint16(b[2:]) != 0 {
			return nil, badBinary(kind, "unused header field is not 0")
		}
		width := 4
		if kind == kindHalfvec {
			width = 2
		}
		if len(b) != 4+width*dim {
			return nil, badBinary(kind, "%d bytes for dimension %d", len(b), dim)
		}
		out := make([]float32, dim)
		for i := range out {
			if kind == kindVector {
				out[i] = math.Float32frombits(binary.BigEndian.Uint32(b[4+4*i:]))
			} else {
				out[i] = halfToFloat32(binary.BigEndian.Uint16(b[4+2*i:]))
			}
		}
		if kind == kindVector {
			v := vector.Vector(out)
			return v, v.Validate(dim)
		}
		h := vector.HalfVector(out)
		return h, h.Validate(dim)
	case kindSparsevec:
		if len(b) < 12 {
			return nil, badBinary(kind, "%d bytes, need a 12-byte header", len(b))
		}
		// Compare as uint32 first: a header with the top bit set would be
		// negative as int32, and both fields are bounded before any use.
		dimU := binary.BigEndian.Uint32(b)
		nnzU := binary.BigEndian.Uint32(b[4:])
		if dimU < 1 || dimU > vector.MaxSparseDim {
			return nil, badBinary(kind, "dimension %d outside 1..%d", dimU, vector.MaxSparseDim)
		}
		if nnzU > vector.MaxSparseNNZ {
			return nil, badBinary(kind, "%d entries exceed the limit of %d", nnzU, vector.MaxSparseNNZ)
		}
		if binary.BigEndian.Uint32(b[8:]) != 0 {
			return nil, badBinary(kind, "unused header field is not 0")
		}
		dim, nnz := int(dimU), int(nnzU)
		if len(b) != 12+8*nnz {
			return nil, badBinary(kind, "%d bytes for %d entries", len(b), nnz)
		}
		s := vector.SparseVector{Dim: dim, Indices: make([]int32, nnz), Values: make([]float32, nnz)}
		for i := range s.Indices {
			idx := binary.BigEndian.Uint32(b[12+4*i:])
			if idx > math.MaxInt32 || idx >= dimU {
				return nil, badBinary(kind, "index %d outside [0, %d)", idx, dim)
			}
			s.Indices[i] = int32(idx)
			s.Values[i] = math.Float32frombits(binary.BigEndian.Uint32(b[12+4*nnz+4*i:]))
		}
		return s, s.Validate(s.Dim)
	case kindBit:
		if len(b) < 4 {
			return nil, badBinary(kind, "%d bytes, need a 4-byte length", len(b))
		}
		nU := binary.BigEndian.Uint32(b)
		if nU < 1 || nU > vector.MaxBits {
			return nil, badBinary(kind, "length %d outside 1..%d", nU, vector.MaxBits)
		}
		n := int(nU)
		if len(b)-4 != (n+7)/8 {
			return nil, badBinary(kind, "%d data bytes for %d bits", len(b)-4, n)
		}
		x := vector.Bits{Len: n, Data: append([]byte(nil), b[4:]...)}
		return x, x.Validate(x.Len)
	}
	return nil, fmt.Errorf("pgxdriver: unknown vector kind %d", kind)
}

// float32ToHalf converts f to IEEE 754 binary16, rounding to nearest even.
// ok is false when the rounded value overflows the finite range.
func float32ToHalf(f float32) (h uint16, ok bool) {
	bits := math.Float32bits(f)
	sign := uint16(bits>>16) & 0x8000
	exp := int((bits>>23)&0xff) - 127 + 15
	mant := bits & 0x7fffff
	switch {
	case exp >= 0x1f:
		return 0, false
	case exp <= 0:
		if exp < -10 {
			return sign, true // underflows to zero
		}
		mant |= 0x800000
		shift := uint(14 - exp)
		half := uint16(mant >> shift) // #nosec G115 -- mant < 2^24 and shift = 14-exp is in 14..24 here, so the result is < 2^10
		rem := mant & (1<<shift - 1)
		mid := uint32(1) << (shift - 1)
		if rem > mid || (rem == mid && half&1 == 1) {
			half++
		}
		return sign | half, true
	}
	half := uint16(exp)<<10 | uint16(mant>>13)
	rem := mant & 0x1fff
	if rem > 0x1000 || (rem == 0x1000 && half&1 == 1) {
		half++
	}
	if half&0x7fff >= 0x7c00 {
		return 0, false
	}
	return sign | half, true
}

// halfToFloat32 widens an IEEE 754 binary16 value.
func halfToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1f
	mant := uint32(h & 0x3ff)
	switch exp {
	case 0:
		if mant == 0 {
			return math.Float32frombits(sign)
		}
		e := uint32(127 - 15 + 1)
		for mant&0x400 == 0 {
			mant <<= 1
			e--
		}
		return math.Float32frombits(sign | e<<23 | (mant&0x3ff)<<13)
	case 0x1f:
		return math.Float32frombits(sign | 0x7f800000 | mant<<13)
	}
	return math.Float32frombits(sign | (exp+127-15)<<23 | mant<<13)
}

// convertUUIDArrayArgs sends a []types.UUID as a uuid[] text literal. pgx
// cannot encode the elements (types.UUID is a driver.Valuer, which pgx only
// honours for scalars), and an unprepared exec has no registered Go type for
// the slice. The server types the text literal from the other operand, so it
// binds as uuid[] either way. A nil slice stays nil and binds as NULL; the
// input slice is not modified.
func convertUUIDArrayArgs(args []any) []any {
	var out []any
	for i, a := range args {
		us, ok := a.([]types.UUID)
		if !ok || us == nil {
			continue
		}
		parts := make([]string, len(us))
		for j, u := range us {
			parts[j] = u.String()
		}
		if out == nil {
			out = append([]any(nil), args...)
		}
		out[i] = "{" + strings.Join(parts, ",") + "}"
	}
	if out == nil {
		return args
	}
	return out
}
