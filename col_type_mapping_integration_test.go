// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
	"github.com/azrtydxb/momo/vector"
)

// liveCase round-trips the edge values of one documented row through a column
// of the given PostgreSQL type.
type liveCase func(t *testing.T, db *momo.DB, colType string)

var tmTableSeq int

// roundTripAll stores each want in colType, reads them back in insertion order
// and compares with eq (reflect.DeepEqual when nil).
func roundTripAll[T any](eq func(got, want T) bool, wants ...T) liveCase {
	return func(t *testing.T, db *momo.DB, colType string) {
		t.Helper()
		ctx := context.Background()
		tmTableSeq++
		tbl := fmt.Sprintf("tm_%d", tmTableSeq)
		ws := wants
		if db.Dialect().Name == dialect.SQLite {
			// SQLite rejects NaN (ErrValidation); TestTypeMappingNaNSQLite pins it.
			ws = slices.DeleteFunc(slices.Clone(wants), func(w T) bool {
				v := reflect.ValueOf(w)
				return (v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64) && math.IsNaN(v.Float())
			})
		}
		if _, err := momo.Exec(ctx, db, "CREATE TABLE "+tbl+" (id int PRIMARY KEY, v "+colType+")"); err != nil {
			t.Fatalf("create %s: %v", colType, err)
		}
		for i, w := range ws {
			if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (?, ?)", i, w); err != nil {
				t.Fatalf("insert %#v into %s: %v", w, colType, err)
			}
		}
		got, err := momo.Scalars[T](ctx, db, "SELECT v FROM "+tbl+" ORDER BY id")
		if err != nil {
			t.Fatalf("select %s: %v", colType, err)
		}
		if len(got) != len(ws) {
			t.Fatalf("%s: got %d rows, want %d", colType, len(got), len(ws))
		}
		for i := range ws {
			same := reflect.DeepEqual(got[i], ws[i])
			if eq != nil {
				same = eq(got[i], ws[i])
			}
			if !same {
				t.Errorf("%s: stored %#v, read back %#v", colType, ws[i], got[i])
			}
		}
	}
}

func f64eq(a, b float64) bool { return a == b || (math.IsNaN(a) && math.IsNaN(b)) }
func f32eq(a, b float32) bool {
	return a == b || (a != a && b != b) //nolint:staticcheck // NaN check on float32
}
func timeEq(a, b time.Time) bool { return a.Equal(b) }
func bytesEq(a, b []byte) bool   { return string(a) == string(b) }

type tmDoc struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func tmUUID(t *testing.T, s string) types.UUID {
	t.Helper()
	u, err := types.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func sptr(s string) *string { return &s }

// liveCases holds one liveCase per documented row, keyed by the row's Go type.
// Edge values: type extremes, NaN and Inf, pre-1970 and non-UTC times, empty
// values, and characters that need escaping.
func liveCases(t *testing.T) map[string]liveCase {
	pre1970 := time.Date(1969, 7, 20, 20, 17, 40, 0, time.UTC)
	ist := time.Date(2026, 1, 1, 12, 0, 0, 123456000, time.FixedZone("IST", 5*3600+1800))
	return map[string]liveCase{
		"int":     roundTripAll[int](nil, math.MaxInt, math.MinInt, 0),
		"int8":    roundTripAll[int8](nil, math.MaxInt8, math.MinInt8),
		"int16":   roundTripAll[int16](nil, math.MaxInt16, math.MinInt16),
		"int32":   roundTripAll[int32](nil, math.MaxInt32, math.MinInt32),
		"int64":   roundTripAll[int64](nil, math.MaxInt64, math.MinInt64),
		"float32": roundTripAll(f32eq, math.MaxFloat32, math.SmallestNonzeroFloat32, float32(math.NaN()), float32(math.Inf(-1))),
		"float64": roundTripAll(f64eq, math.MaxFloat64, math.SmallestNonzeroFloat64, math.NaN(), math.Inf(1)),
		"bool":    roundTripAll[bool](nil, true, false),
		"string": roundTripAll[string](nil, "", "héllo 日本 \U0001F600 \"q\" 'q' \\ ?",
			strings.Repeat("x", 100000)),
		"string with size:N": roundTripAll[string](nil, "", "abc"),
		"[]byte":             roundTripAll(bytesEq, []byte{0, 1, 255}, []byte{}),
		"time.Time":          roundTripAll(timeEq, pre1970, ist, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)),
		"types.UUID": roundTripAll[types.UUID](nil,
			tmUUID(t, "00000000-0000-0000-0000-000000000000"), tmUUID(t, "ffffffff-ffff-4fff-bfff-ffffffffffff")),
		"types.Date": roundTripAll(func(a, b types.Date) bool { return a.Time.Equal(b.Time) },
			types.Date{Time: time.Date(1969, 12, 31, 0, 0, 0, 0, time.UTC)},
			types.Date{Time: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)}),
		"types.JSON[T]": roundTripAll[types.JSON[tmDoc]](nil,
			types.JSON[tmDoc]{V: tmDoc{Name: `q"uote\`, Tags: []string{"a", "b"}}},
			types.JSON[tmDoc]{V: tmDoc{Name: "", Tags: []string{}}}),
		"vector.Vector":              roundTripAll[vector.Vector](nil, vector.Vector{1, -2.5, 3}, vector.Vector{math.MaxFloat32, 0, math.SmallestNonzeroFloat32}),
		"vector.HalfVector":          roundTripAll[vector.HalfVector](nil, vector.HalfVector{1, -2.5, 0.5}, vector.HalfVector{65504, 0, -65504}),
		"vector.SparseVector":        roundTripAll[vector.SparseVector](nil, vector.SparseVector{Dim: 3, Indices: []int32{0, 2}, Values: []float32{1.5, -2}}),
		"vector.Bits":                roundTripAll[vector.Bits](nil, vector.Bits{Len: 3, Data: []byte{0xA0}}),
		"vector.Vector (no dim)":     roundTripAll[vector.Vector](nil, vector.Vector{1, -2.5, 3}, vector.Vector{1, 2}),
		"vector.HalfVector (no dim)": roundTripAll[vector.HalfVector](nil, vector.HalfVector{1, -2.5, 0.5}, vector.HalfVector{4, 5}),
		"types.TimeOfDay": roundTripAll[types.TimeOfDay](nil,
			types.TimeOfDay{Nanos: 0}, types.TimeOfDay{Nanos: 86399999999000}),
		"types.Hstore": roundTripAll[types.Hstore](nil,
			types.Hstore{"k": sptr("v"), "n": nil, `q"=>\`: sptr(`a"b\c`)}, types.Hstore{}),
		"types.TSVector": roundTripAll[types.TSVector](nil, types.TSVector("'cat':3 'fat':2"),
			types.TSVector(`'a''b':1A 'c\\d':2B,3 'plain':4`), types.TSVector("")),
		"types.TSQuery": roundTripAll[types.TSQuery](nil, types.TSQuery("'fat' & 'cat'"),
			types.TSQuery(`'a''b' & 'c\\d':*`), types.TSQuery("")),
		"time.Time (timestamp)": roundTripAll(timeEq, pre1970, time.Date(2026, 1, 1, 12, 0, 0, 123456000, time.UTC)),
		"[]string": roundTripAll[[]string](nil,
			[]string{"a", "b,c", `w"q`, "NULL", ""}, []string{}),
		"[]int16":   roundTripAll[[]int16](nil, []int16{math.MaxInt16, math.MinInt16}, []int16{}),
		"[]int32":   roundTripAll[[]int32](nil, []int32{math.MaxInt32, math.MinInt32}, []int32{}),
		"[]int64":   roundTripAll[[]int64](nil, []int64{math.MaxInt64, math.MinInt64}, []int64{}),
		"[]bool":    roundTripAll[[]bool](nil, []bool{true, false}, []bool{}),
		"[]float32": roundTripAll[[]float32](nil, []float32{math.MaxFloat32, -1.5}, []float32{}),
		"[]types.UUID": roundTripAll[[]types.UUID](nil,
			[]types.UUID{{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0xff}, {}}, []types.UUID{}, []types.UUID(nil)),
		"[]float64": roundTripAll[[]float64](nil, []float64{math.MaxFloat64, -1.5}, []float64{}),
		"string (decimal)": roundTripAll[string](nil,
			"12345678901234567890.123456789", "-0.000000000000000001", "NaN"),
	}
}

// TestTypeMapping round-trips edge values for every row of the documented
// type-mapping table through a live column of the documented type, on
// PostgreSQL and on SQLite (rows the document marks unsupported or n/a for
// SQLite are skipped there). A row without a live case, or a case without a row, fails.
func TestTypeMapping(t *testing.T) {
	cases := liveCases(t)
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		if d == dialect.Postgres {
			// hstore is a database-wide extension that migrate's drift checks
			// would report as unmanaged: keep it out of the shared database.
			db = testopen.PostgresIn(t, testdb.ScratchDatabase(t, "momo_typemap_"))
			for _, ext := range []string{"vector", "hstore"} {
				if _, err := momo.Exec(ctx, db, "CREATE EXTENSION IF NOT EXISTS "+ext+" SCHEMA public"); err != nil {
					t.Fatalf("create extension %s: %v", ext, err)
				}
			}
		}
		doc := parseDocRows(t)
		for _, r := range mappingRows {
			c, ok := cases[r.goType]
			if !ok {
				t.Errorf("documented row %q has no round-trip case", r.goType)
				continue
			}
			cells, ok := doc[r.goType]
			if !ok {
				t.Errorf("documented row %q is not in %s", r.goType, typeMappingDoc)
				continue
			}
			// The column type comes from the document itself.
			colType := cells[0]
			if d == dialect.SQLite {
				colType = cells[1]
				if colType == "unsupported" || colType == "n/a" {
					// The document says no SQLite column is generated for this
					// row; TestTypeMappingDoc checks the generator agrees.
					continue
				}
			}
			colType = strings.ReplaceAll(colType, "(N)", "(3)")
			t.Run(r.goType, func(t *testing.T) { c(t, db, colType) })
		}
		for k := range cases {
			found := false
			for _, r := range mappingRows {
				found = found || r.goType == k
			}
			if !found {
				t.Errorf("round-trip case %q has no documented row", k)
			}
		}
	})
}

// TestTypeMappingTimestamps pins the time semantics the document states:
// timestamptz keeps the instant but not the zone, microsecond precision,
// timestamp (no zone) stores the wall clock of the UTC instant.
func TestTypeMappingTimestamps(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_ts (tz timestamptz, ts timestamp)"); err != nil {
		t.Fatal(err)
	}
	ist := time.Date(2026, 1, 1, 12, 0, 0, 123456789, time.FixedZone("IST", 5*3600+1800))
	if _, err := momo.Exec(ctx, db, "INSERT INTO tm_ts VALUES (?, ?)", ist, ist.UTC()); err != nil {
		t.Fatal(err)
	}
	txt, err := momo.Scalars[string](ctx, db, "SELECT to_char(tz AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS.US') || ' | ' || to_char(ts, 'YYYY-MM-DD HH24:MI:SS.US') FROM tm_ts")
	if err != nil || len(txt) != 1 {
		t.Fatalf("select: %v", err)
	}
	// 12:00:00 at +05:30 is 06:30:00 UTC; nanoseconds are cut to microseconds.
	if want := "2026-01-01 06:30:00.123456 | 2026-01-01 06:30:00.123456"; txt[0] != want {
		t.Fatalf("stored %q, want %q", txt[0], want)
	}
	got, err := momo.Scalars[time.Time](ctx, db, "SELECT tz FROM tm_ts")
	if err != nil || len(got) != 1 || !got[0].Equal(ist.Truncate(time.Microsecond)) {
		t.Fatalf("timestamptz read back %v, %v", got, err)
	}
}

// TestTypeMappingNaN covers the float64 edge on PostgreSQL: NaN is a legal
// double precision value and round-trips; the literal form scans too.
func TestTypeMappingNaN(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	got, err := momo.Scalars[float64](ctx, db, "SELECT 'NaN'::float8")
	if err != nil || len(got) != 1 || !math.IsNaN(got[0]) {
		t.Fatalf("SELECT 'NaN'::float8 = %v, %v", got, err)
	}
	back, err := momo.Scalars[float64](ctx, db, "SELECT ?::float8", math.NaN())
	if err != nil || len(back) != 1 || !math.IsNaN(back[0]) {
		t.Fatalf("bound NaN = %v, %v", back, err)
	}
}

// TestTypeMappingNaNSQLite pins the SQLite edge the document states: SQLite
// has no NaN, so binding one is ErrValidation (float32, float64 and pointers to
// them, on the pool and inside a transaction), nothing is stored, and +Inf and
// -Inf stay legal REAL values.
func TestTypeMappingNaNSQLite(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_nan (id INTEGER PRIMARY KEY, v REAL)"); err != nil {
		t.Fatal(err)
	}
	f32, f64 := float32(math.NaN()), math.NaN()
	for i, v := range []any{math.NaN(), float32(math.NaN()), &f64, &f32} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO tm_nan (id, v) VALUES (?, ?)", i, v); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("insert %T NaN: err = %v, want ErrValidation", v, err)
		}
	}
	if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := momo.Exec(ctx, tx, "INSERT INTO tm_nan (id, v) VALUES (?, ?)", 9, math.NaN())
		return err
	}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("insert NaN in a transaction: err = %v, want ErrValidation", err)
	}
	for i, v := range []float64{math.Inf(1), math.Inf(-1)} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO tm_nan (id, v) VALUES (?, ?)", i, v); err != nil {
			t.Fatalf("insert %v: %v", v, err)
		}
	}
	got, err := momo.Scalars[float64](ctx, db, "SELECT v FROM tm_nan ORDER BY id")
	if err != nil || len(got) != 2 || !math.IsInf(got[0], 1) || !math.IsInf(got[1], -1) {
		t.Fatalf("rows = %v, %v; want only +Inf and -Inf", got, err)
	}
}

// TestTypeMappingNullable pins the nullability rule: pointers and sql.Null*
// map to the type they wrap, nil is NULL, and scanning NULL into a plain
// value is an error rather than a silent zero.
func TestTypeMappingNullable(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_null (id int PRIMARY KEY, i bigint, s text, t timestamptz)"); err != nil {
		t.Fatal(err)
	}
	var np *int64
	now := time.Date(1969, 7, 20, 20, 17, 40, 0, time.UTC)
	if _, err := momo.Exec(ctx, db, "INSERT INTO tm_null VALUES (1, ?, ?, ?), (2, ?, ?, ?)",
		np, sql.NullString{}, sql.NullTime{}, int64(math.MaxInt64), sql.NullString{String: "x", Valid: true}, sql.NullTime{Time: now, Valid: true}); err != nil {
		t.Fatal(err)
	}
	ptrs, err := momo.Scalars[*int64](ctx, db, "SELECT i FROM tm_null ORDER BY id")
	if err != nil || len(ptrs) != 2 || ptrs[0] != nil || ptrs[1] == nil || *ptrs[1] != math.MaxInt64 {
		t.Fatalf("*int64 = %v, %v", ptrs, err)
	}
	ns, err := momo.Scalars[sql.NullString](ctx, db, "SELECT s FROM tm_null ORDER BY id")
	if err != nil || ns[0].Valid || ns[1] != (sql.NullString{String: "x", Valid: true}) {
		t.Fatalf("sql.NullString = %v, %v", ns, err)
	}
	nt, err := momo.Scalars[sql.NullTime](ctx, db, "SELECT t FROM tm_null ORDER BY id")
	if err != nil || nt[0].Valid || !nt[1].Valid || !nt[1].Time.Equal(now) {
		t.Fatalf("sql.NullTime = %v, %v", nt, err)
	}
	if _, err := momo.Scalars[int64](ctx, db, "SELECT i FROM tm_null WHERE id = 1"); err == nil {
		t.Fatal("scanning NULL into int64 succeeded; want an error")
	}
}

// TestTypeMappingRejections pins the documented hard limits.
func TestTypeMappingRejections(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_rej (s text, v character varying(3), i smallint)"); err != nil {
		t.Fatal(err)
	}
	for name, stmt := range map[string]struct {
		sql string
		arg any
	}{
		"NUL in text":        {"INSERT INTO tm_rej (s) VALUES (?)", "a\x00b"},
		"varchar over size":  {"INSERT INTO tm_rej (v) VALUES (?)", "abcd"},
		"int64 out of int16": {"INSERT INTO tm_rej (i) VALUES (?)", int64(math.MaxInt16) + 1},
		"uint64 above int64": {"INSERT INTO tm_rej (i) VALUES (?)", uint64(math.MaxUint64)},
	} {
		if _, err := momo.Exec(ctx, db, stmt.sql, stmt.arg); err == nil {
			t.Errorf("%s: insert succeeded, want an error", name)
		}
	}
}
