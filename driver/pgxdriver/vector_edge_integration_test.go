// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/vector"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func acquireCodecConn(t *testing.T) (context.Context, *pgxpool.Conn) {
	t.Helper()
	ctx := context.Background()
	pool := openCodecPool(t, testdb.PostgresURL(t))
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Release)
	return ctx, conn
}

// serverSend returns the bytes pgvector's own send function produces for the
// text literal, or the server error.
func serverSend(ctx context.Context, conn *pgxpool.Conn, fn, typ, literal string) ([]byte, error) {
	var out []byte
	q := fmt.Sprintf("SELECT %s($1::text::%s)", fn, typ)
	err := conn.QueryRow(ctx, q, pgx.QueryExecModeExec, literal).Scan(&out)
	return out, err
}

func floatText(x float32) string { return strconv.FormatFloat(float64(x), 'g', -1, 32) }

// TestHalfvecEdgeValuesMatchServer compares the codec with pgvector for the
// edges of float16: the largest value, subnormals, negative zero, underflow,
// and round-to-nearest-even ties. For 65504 < x < 65520 pgvector rounds down
// to 65504 and so does the codec; from 65520 on both reject the value.
func TestHalfvecEdgeValuesMatchServer(t *testing.T) {
	ctx, conn := acquireCodecConn(t)
	accepted := []float32{
		65504, -65504, 6e-8, 5.96e-8, float32(math.Copysign(0, -1)), 0, 1e-9,
		65519, 65519.9, 65504.5, 65512, -65519,
		1.00048828125,               // 1 + 2^-11: tie, rounds to even (1)
		1.00146484375,               // 1 + 3*2^-11: tie, rounds to even (1 + 2^-9)
		1.0005,                      // just above the tie
		2.98e-8, 2.9802322e-8, 3e-8, // around half the smallest subnormal
		6.1035156e-05,                 // smallest normal
		6.0975552e-05,                 // largest subnormal
		0.1, 3.14159, -2.5e-5, 1000.7, // ordinary rounding
	}
	rejected := []float32{65520, 65520.5, 65536, 1e6, -65520, -1e9}
	for _, x := range accepted {
		lit := "[" + floatText(x) + "]"
		want, err := serverSend(ctx, conn, "halfvec_send", "halfvec", lit)
		if err != nil {
			t.Errorf("%v: server rejects %s: %v", x, lit, err)
			continue
		}
		got, err := encodeBinary(kindHalfvec, vector.HalfVector{x}, nil)
		if err != nil {
			t.Errorf("%v: codec rejects a value the server accepts: %v", x, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%v: encode % x, server % x", x, got, want)
		}
		back, err := decodeBinary(kindHalfvec, want)
		if err != nil {
			t.Errorf("%v: decode: %v", x, err)
			continue
		}
		// Server round trip: bind the codec's bytes, read the value back.
		var rt vector.HalfVector
		if err := conn.QueryRow(ctx, "SELECT $1::halfvec", pgx.QueryExecModeDescribeExec, vector.HalfVector{x}).Scan(&rt); err != nil {
			t.Errorf("%v: round trip: %v", x, err)
			continue
		}
		b := back.(vector.HalfVector)
		if len(rt) != 1 || math.Float32bits(rt[0]) != math.Float32bits(b[0]) {
			t.Errorf("%v: round trip %v (%#x), want %v (%#x)", x, rt, math.Float32bits(rt[0]), b[0], math.Float32bits(b[0]))
		}
	}
	for _, x := range rejected {
		lit := "[" + floatText(x) + "]"
		if _, err := serverSend(ctx, conn, "halfvec_send", "halfvec", lit); err == nil {
			t.Errorf("%v: server accepts %s", x, lit)
		}
		if _, err := encodeBinary(kindHalfvec, vector.HalfVector{x}, nil); !errors.Is(err, vector.ErrInvalidVector) {
			t.Errorf("%v: encode err = %v, want ErrInvalidVector", x, err)
		}
		if _, err := (vector.HalfVector{x}).Value(); !errors.Is(err, vector.ErrInvalidVector) {
			t.Errorf("%v: Value err = %v, want ErrInvalidVector", x, err)
		}
	}
}

func sparseSendText(dim, nnz int) string {
	parts := make([]string, nnz)
	for i := range parts {
		parts[i] = fmt.Sprintf("%d:1", i+1)
	}
	return "{" + strings.Join(parts, ",") + "}/" + strconv.Itoa(dim)
}

func sparseOf(dim, nnz int) vector.SparseVector {
	s := vector.SparseVector{Dim: dim, Indices: make([]int32, nnz), Values: make([]float32, nnz)}
	for i := range s.Indices {
		s.Indices[i] = int32(i)
		s.Values[i] = 1
	}
	return s
}

// TestSparsevecLimitsMatchServer checks dim 1..1e9 and nnz <= 16000 on the
// text and the binary path against pgvector itself.
func TestSparsevecLimitsMatchServer(t *testing.T) {
	ctx, conn := acquireCodecConn(t)
	cases := []struct {
		name string
		s    vector.SparseVector
		text string
		ok   bool
	}{
		{"dim 1", sparseOf(1, 1), sparseSendText(1, 1), true},
		{"dim 1e9", vector.SparseVector{Dim: vector.MaxSparseDim, Indices: []int32{vector.MaxSparseDim - 1}, Values: []float32{2}}, "{1000000000:2}/1000000000", true},
		{"dim 1e9 empty", vector.SparseVector{Dim: vector.MaxSparseDim}, "{}/1000000000", true},
		{"nnz 16000", sparseOf(1_000_000, 16000), sparseSendText(1_000_000, 16000), true},
		{"dim 1e9+1", vector.SparseVector{Dim: vector.MaxSparseDim + 1}, "{}/1000000001", false},
		{"dim 0", vector.SparseVector{}, "{}/0", false},
		{"nnz 16001", sparseOf(1_000_000, 16001), sparseSendText(1_000_000, 16001), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want, serr := serverSend(ctx, conn, "sparsevec_send", "sparsevec", c.text)
			if (serr == nil) != c.ok {
				t.Fatalf("server: err = %v, want ok=%v", serr, c.ok)
			}
			// Binary encode and decode.
			got, err := encodeBinary(kindSparsevec, c.s, nil)
			if c.ok {
				if err != nil {
					t.Fatalf("encode: %v", err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("encode differs from the server's send bytes (%d vs %d)", len(got), len(want))
				}
				back, err := decodeBinary(kindSparsevec, want)
				if err != nil {
					t.Fatalf("decode: %v", err)
				}
				if bs := back.(vector.SparseVector); bs.Dim != c.s.Dim || len(bs.Indices) != len(c.s.Indices) {
					t.Fatalf("decode = dim %d nnz %d", bs.Dim, len(bs.Indices))
				}
				// Server round trip, binary then text.
				for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeDescribeExec, pgx.QueryExecModeExec} {
					var rt vector.SparseVector
					if err := conn.QueryRow(ctx, "SELECT $1::sparsevec", mode, c.s).Scan(&rt); err != nil {
						t.Fatalf("round trip mode %v: %v", mode, err)
					}
					if rt.Dim != c.s.Dim || len(rt.Indices) != len(c.s.Indices) {
						t.Fatalf("round trip mode %v = dim %d nnz %d", mode, rt.Dim, len(rt.Indices))
					}
				}
			} else if !errors.Is(err, vector.ErrInvalidVector) {
				t.Fatalf("encode err = %v, want ErrInvalidVector", err)
			}
			// Text path.
			_, verr := c.s.Value()
			if (verr == nil) != c.ok {
				t.Fatalf("Value err = %v, want ok=%v", verr, c.ok)
			}
			var scanned vector.SparseVector
			if serr := scanned.Scan(c.text); (serr == nil) != c.ok {
				t.Fatalf("Scan(%.40q) err = %v, want ok=%v", c.text, serr, c.ok)
			}
		})
	}
}

// TestVectorTextBatch sends a vector through the batch path under VectorText on
// a pool with ConfigureVectors: the argument must travel as text.
func TestVectorTextBatch(t *testing.T) {
	ctx := context.Background()
	pool := openCodecPool(t, testdb.PostgresURL(t))
	for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeExec} {
		drv := New(pool, WithVectorFormat(VectorText), WithExecMode(mode))
		b, ok := drv.(momo.Batcher)
		if !ok {
			t.Fatal("driver is not a Batcher")
		}
		rows, err := b.SendBatch(ctx, []momo.Compiled{
			{SQL: "SELECT $1::vector::text", Args: []any{&vector.Vector{1, 2}}},
			{SQL: "SELECT $1::sparsevec::text", Args: []any{vector.SparseVector{Dim: 4, Indices: []int32{3}, Values: []float32{1}}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"[1,2]", "{4:1}/4"}
		for i, r := range rows {
			if !r.Next() {
				t.Fatalf("stmt %d: no row: %v", i, r.Err())
			}
			var s string
			if err := r.Scan(&s); err != nil || s != want[i] {
				t.Errorf("stmt %d = %q, %v, want %q", i, s, err, want[i])
			}
			_ = r.Close()
		}
	}
}

// TestVectorsInNonPublicExtensionSchema installs pgvector into a schema other
// than public, in a database created for this test, and checks that the OID
// lookup and the binary codec still work. It needs CREATEDB and CREATE
// EXTENSION rights, otherwise it skips.
func TestVectorsInNonPublicExtensionSchema(t *testing.T) {
	ctx := context.Background()
	base := testdb.PostgresURL(t)
	cfg, err := pgx.ParseConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "momo_vec_" + hex.EncodeToString(suffix[:])
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Skipf("cannot create a test database (%v); skipping the non-public schema test", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})
	pcfg, err := pgxpool.ParseConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	pcfg.ConnConfig.Database = name
	delete(pcfg.ConnConfig.RuntimeParams, "search_path")
	setup, err := pgx.ConnectConfig(ctx, pcfg.ConnConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"CREATE SCHEMA ext", "CREATE EXTENSION vector WITH SCHEMA ext"} {
		if _, err := setup.Exec(ctx, q); err != nil {
			_ = setup.Close(ctx)
			t.Skipf("cannot install pgvector into a custom schema (%v); skipping", err)
		}
	}
	_ = setup.Close(ctx)

	ConfigureVectors(pcfg)
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closePool(t, pool)
	in := codecInput()
	for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeDescribeExec, pgx.QueryExecModeExec} {
		drv := New(pool, WithVectorFormat(VectorBinary), WithExecMode(mode))
		rows, err := drv.Query(ctx, "SELECT $1::ext.vector, $2::ext.halfvec, $3::ext.sparsevec", []any{in.v, in.h, in.s})
		if err != nil {
			t.Fatalf("mode %v: %v", mode, err)
		}
		if !rows.Next() {
			t.Fatalf("mode %v: no row: %v", mode, rows.Err())
		}
		var out codecRow
		if err := rows.Scan(&out.v, &out.h, &out.s); err != nil {
			t.Fatalf("mode %v: scan: %v", mode, err)
		}
		_ = rows.Close()
		if fmt.Sprint(out.v, out.h, out.s) != fmt.Sprint(in.v, in.h, in.s) {
			t.Errorf("mode %v: got %+v, want %+v", mode, out, in)
		}
	}
}
