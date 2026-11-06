// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"runtime"
	"testing"

	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/vector"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type codecRow struct {
	v vector.Vector
	h vector.HalfVector
	s vector.SparseVector
	b vector.Bits
}

func codecInput() codecRow {
	return codecRow{
		v: vector.Vector{1, 2.5, -3},
		h: vector.HalfVector{0.5, 1, 2},
		s: vector.SparseVector{Dim: 5, Indices: []int32{0, 4}, Values: []float32{1, -2}},
		b: vector.Bits{Len: 9, Data: []byte{0xAA, 0x80}},
	}
}

func openCodecPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	ConfigureVectors(cfg)
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	return pool
}

const codecDDL = "CREATE TABLE codec_t (id int, v vector(3), h halfvec(3), s sparsevec(5), b bit(9))"

// readCodecRow inserts in into a fresh codec_t and reads it back.
func readCodecRow(t *testing.T, format VectorFormat, mode pgx.QueryExecMode, in codecRow) codecRow {
	t.Helper()
	ctx := context.Background()
	pool := openCodecPool(t, testdb.PostgresURL(t))
	drv := New(pool, WithVectorFormat(format), WithExecMode(mode))
	if _, err := drv.Exec(ctx, codecDDL, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := drv.Exec(ctx, "INSERT INTO codec_t VALUES (1, $1, $2, $3, $4)", []any{in.v, in.h, in.s, in.b}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	rows, err := drv.Query(ctx, "SELECT v, h, s, b FROM codec_t WHERE id = 1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("no row: %v", rows.Err())
	}
	var out codecRow
	if err := rows.Scan(&out.v, &out.h, &out.s, &out.b); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

func TestVectorBinaryCodec(t *testing.T) {
	in := codecInput()
	modes := []struct {
		name string
		mode pgx.QueryExecMode
	}{
		{"exec", pgx.QueryExecModeExec},
		{"describe", pgx.QueryExecModeDescribeExec},
		{"cache", pgx.QueryExecModeCacheStatement},
	}
	formats := []struct {
		name   string
		format VectorFormat
	}{{"text", VectorText}, {"binary", VectorBinary}}
	var first *codecRow
	for _, f := range formats {
		for _, m := range modes {
			t.Run(f.name+"/"+m.name, func(t *testing.T) {
				got := readCodecRow(t, f.format, m.mode, in)
				if !reflect.DeepEqual(got, in) {
					t.Fatalf("got %+v, want %+v", got, in)
				}
				if first == nil {
					first = &got
				} else if !reflect.DeepEqual(*first, got) {
					t.Fatalf("formats disagree: %+v vs %+v", *first, got)
				}
			})
		}
	}
}

// TestVectorBinaryWireMatchesPgvector compares the codec's binary bytes with
// pgvector's own *_send output, and decodes pgvector's bytes back.
func TestVectorBinaryWireMatchesPgvector(t *testing.T) {
	ctx := context.Background()
	pool := openCodecPool(t, testdb.PostgresURL(t))
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	in := codecInput()
	cases := []struct {
		kind  vectorKind
		value any
		send  string
	}{
		{kindVector, in.v, "SELECT vector_send('[1,2.5,-3]'::vector)"},
		{kindHalfvec, in.h, "SELECT halfvec_send('[0.5,1,2]'::halfvec)"},
		{kindSparsevec, in.s, "SELECT sparsevec_send('{1:1,5:-2}/5'::sparsevec)"},
		{kindBit, in.b, "SELECT bit_send(B'101010101')"},
	}
	for _, c := range cases {
		var want []byte
		if err := conn.QueryRow(ctx, c.send, pgx.QueryExecModeExec).Scan(&want); err != nil {
			t.Fatalf("%s: %v", c.send, err)
		}
		got, err := encodeBinary(c.kind, c.value, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: encode % x, pgvector % x", c.send, got, want)
		}
		back, err := decodeBinary(c.kind, want)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(back, c.value) {
			t.Errorf("%s: decode %+v, want %+v", c.send, back, c.value)
		}
	}
}

// TestVectorBinarySQLComparison lets the server compare the bound binary
// parameter with a literal.
func TestVectorBinarySQLComparison(t *testing.T) {
	ctx := context.Background()
	in := codecInput()
	drv := New(openCodecPool(t, testdb.PostgresURL(t)),
		WithVectorFormat(VectorBinary), WithExecMode(pgx.QueryExecModeDescribeExec))
	cases := []struct {
		sql string
		arg any
	}{
		{"SELECT $1::vector = '[1,2.5,-3]'::vector", in.v},
		{"SELECT $1::halfvec = '[0.5,1,2]'::halfvec", in.h},
		{"SELECT $1::sparsevec = '{1:1,5:-2}/5'::sparsevec", in.s},
		{"SELECT $1::bit(9) = B'101010101'", in.b},
	}
	for _, c := range cases {
		rows, err := drv.Query(ctx, c.sql, []any{c.arg})
		if err != nil {
			t.Fatalf("%s: %v", c.sql, err)
		}
		var eq bool
		if !rows.Next() {
			t.Fatalf("%s: no row", c.sql)
		}
		if err := rows.Scan(&eq); err != nil || !eq {
			t.Errorf("%s: eq=%v err=%v", c.sql, eq, err)
		}
		_ = rows.Close()
	}
}

func TestVectorCopyFrom(t *testing.T) {
	ctx := context.Background()
	in := codecInput()
	drv := New(openCodecPool(t, testdb.PostgresURL(t)))
	cp, ok := drv.(interface {
		CopyFrom(context.Context, string, []string, [][]any) (int64, error)
	})
	if !ok {
		t.Fatal("driver is not a Copier")
	}
	if _, err := drv.Exec(ctx, codecDDL, nil); err != nil {
		t.Fatal(err)
	}
	n, err := cp.CopyFrom(ctx, "codec_t", []string{"id", "v", "h", "s", "b"}, [][]any{{1, in.v, in.h, in.s, in.b}})
	if err != nil || n != 1 {
		t.Fatalf("copy: n=%d err=%v", n, err)
	}
	rows, err := drv.Query(ctx, "SELECT v, h, s, b FROM codec_t", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out codecRow
	if !rows.Next() {
		t.Fatal("no row")
	}
	if err := rows.Scan(&out.v, &out.h, &out.s, &out.b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestVectorOIDLookupMissingExtension(t *testing.T) {
	ctx := context.Background()
	pool := openCodecPool(t, testdb.PostgresURL(t))
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	oid, found, err := lookupExtensionType(ctx, conn.Conn(), "no_such_extension", "vector")
	if err != nil || found || oid != 0 {
		t.Fatalf("oid=%d found=%v err=%v, want not found without error", oid, found, err)
	}
	if _, found, err := lookupExtensionType(ctx, conn.Conn(), "vector", "vector"); err != nil || !found {
		t.Fatalf("vector type: found=%v err=%v", found, err)
	}
}

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
