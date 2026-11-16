// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/gen/typemap"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
	"github.com/azrtydxb/momo/vector"
)

// docMySQLRowRE matches a generated mapping row with its fourth cell, the
// MySQL column.
var docMySQLRowRE = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|[^|]*\\|[^|]*\\|([^|]*)\\|$")

// parseDocMySQL returns the MySQL cell of every generated row of the type
// mapping document, keyed by the Go type.
func parseDocMySQL(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(typeMappingDoc)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		m := docMySQLRowRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// docUnsignedRE matches "`uint8` to `tinyint unsigned`" in the document,
// across a line break.
var docUnsignedRE = regexp.MustCompile("`(uint(?:8|16|32|64)?)`\\s+to\\s+`([a-z]+ unsigned)`")

// parseDocUnsigned returns the column type the document names for each
// unsigned Go integer.
func parseDocUnsigned(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(typeMappingDoc)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range docUnsignedRE.FindAllStringSubmatch(string(b), -1) {
		out[m[1]] = m[2]
	}
	return out
}

// mysqlLiveCases is liveCases with the edge values of the MySQL column:
// FLOAT and DOUBLE have no NaN or infinity and no denormals, and text and blob
// hold 64 KiB (65535 bytes).
func mysqlLiveCases(t *testing.T) map[string]liveCase {
	cases := liveCases(t)
	// A query without arguments reads FLOAT with six significant digits, so the
	// values here are the nearest float32 to a decimal of six digits or fewer;
	// TestTypeMappingMySQLLimits pins the loss for the full range.
	cases["float32"] = roundTripAll(f32eq, float32(1.5e38), float32(-1.5e38),
		float32(1e-30), float32(-1e-30), float32(0), float32(0.1), float32(123456))
	cases["float64"] = roundTripAll(f64eq, math.MaxFloat64, -math.MaxFloat64,
		2.2250738585072014e-308, 0, 0.1)
	cases["string"] = roundTripAll[string](nil, "", "héllo 日本 \U0001F600 \"q\" 'q' \\ ?",
		strings.Repeat("x", 65535))
	cases["string with size:N"] = roundTripAll[string](nil, "", "abc", "日本語")
	cases["[]byte"] = roundTripAll(bytesEq, []byte{0, 1, 255}, []byte{}, bytes.Repeat([]byte{0xAB}, 65535))
	return cases
}

// tmVec is a model with a vector column, written through the typed insert
// path, which wraps the vector in the server's text-to-vector function.
type tmVec struct {
	ID  int64
	Emb vector.Vector
}

var tmVecMeta = momo.Meta[tmVec]{
	Table:      "tm_vec",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}},
	Ptr:        func(m *tmVec, i int) any { return []any{&m.ID, &m.Emb}[i] },
	Value:      func(m *tmVec, i int) any { return []any{m.ID, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *tmVec) MomoMeta() *momo.Meta[tmVec] { return &tmVecMeta }

// tmVectorMySQL checks the vector.Vector row: MySQL 8.4 has no VECTOR type;
// MySQL 9.7 and MariaDB 11.8 store and read it back.
func tmVectorMySQL(t *testing.T, db *momo.DB, cell string) {
	ctx := context.Background()
	version, err := momo.Scalars[string](ctx, db, "SELECT VERSION()")
	if err != nil || len(version) != 1 {
		t.Fatalf("VERSION(): %v, %v", version, err)
	}
	typ, err := momo.MySQLVectorType(db.Dialect().Flavor, version[0], 3)
	if !strings.EqualFold(strings.ReplaceAll(cell, "(N)", "(3)"), typ) && err == nil {
		t.Fatalf("documented %q, MySQLVectorType = %q", cell, typ)
	}
	if err != nil {
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("MySQLVectorType on %s: %v, want ErrUnsupported", version[0], err)
		}
		if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_vec (id bigint PRIMARY KEY, emb vector(3))"); err == nil {
			t.Fatalf("CREATE TABLE with a vector column succeeded on %s, which has no VECTOR type", version[0])
		}
		return
	}
	if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_vec (id bigint PRIMARY KEY, emb "+typ+" NOT NULL)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	candidates := []tmVec{
		{ID: 1, Emb: vector.Vector{1, -2.5, 3}},
		{ID: 2, Emb: vector.Vector{1e-30, -1e-30, 0}},
		{ID: 3, Emb: vector.Vector{math.Float32frombits(0x00800000), 0, 0}},
		{ID: 4, Emb: vector.Vector{0, 0, 0}},
	}
	if db.Dialect().Flavor == dialect.FlavorMariaDB {
		// MariaDB refuses a component of large magnitude (1e30 and above), so
		// the largest float32 is a MySQL-only edge value.
		for _, big := range []float32{1e30, -1e30, math.MaxFloat32} {
			if _, err := momo.Insert[tmVec](db).Models([]tmVec{{ID: 100, Emb: vector.Vector{big, 0, 0}}}).Exec(ctx); err == nil {
				t.Errorf("MariaDB took a vector component of %v, which the document says it refuses", big)
			}
		}
	} else {
		candidates = append(candidates, tmVec{ID: 5, Emb: vector.Vector{math.MaxFloat32, -math.MaxFloat32, 0}})
	}
	// One row at a time, so a refused value names itself.
	var wants []tmVec
	for _, c := range candidates {
		if _, err := momo.Insert[tmVec](db).Models([]tmVec{c}).Exec(ctx); err != nil {
			t.Errorf("insert %v: %v", c.Emb, err)
			continue
		}
		wants = append(wants, c)
	}
	for _, w := range wants {
		got, err := momo.Select[tmVec](db).Where(momo.NewColumn[int64]("tm_vec", "id", 0).Eq(w.ID)).All(ctx)
		if err != nil || len(got) != 1 {
			t.Fatalf("select id %d: %v, %v", w.ID, got, err)
		}
		for i := range w.Emb {
			if got[0].Emb[i] != w.Emb[i] {
				t.Errorf("id %d: stored %v, read back %v", w.ID, w.Emb, got[0].Emb)
				break
			}
		}
	}
}

// TestTypeMappingMySQL creates a column of each documented MySQL type on
// MySQL 8.4, MySQL 9.7 and MariaDB 11.8 and round-trips the edge values of the
// row through it, then NULL. A row the document marks unsupported has no MySQL
// column; a row with a MySQL type and no case, or the reverse, fails.
func TestTypeMappingMySQL(t *testing.T) {
	cases := mysqlLiveCases(t)
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		doc := parseDocMySQL(t)
		covered := map[string]bool{}
		for _, r := range mappingRows {
			cell, ok := doc[r.goType]
			if r.src != srcGenerator {
				// Codec and manual rows have no generated MySQL column and
				// no fourth cell.
				if ok {
					t.Errorf("row %q is a %s row but %s gives it a MySQL cell", r.goType, r.src, typeMappingDoc)
				}
				continue
			}
			if !ok {
				t.Errorf("generated row %q has no MySQL cell in %s", r.goType, typeMappingDoc)
				continue
			}
			if cell == "unsupported" {
				continue
			}
			covered[r.goType] = true
			if r.goType == "vector.Vector" {
				t.Run(r.goType, func(t *testing.T) { tmVectorMySQL(t, db, cell) })
				continue
			}
			c, ok := cases[r.goType]
			if !ok {
				t.Errorf("documented MySQL row %q has no round-trip case", r.goType)
				continue
			}
			colType := strings.ReplaceAll(cell, "(N)", "(3)")
			t.Run(r.goType, func(t *testing.T) {
				c(t, db, colType)
				// NULL: the same column holds it and reads it as NULL.
				tbl := fmt.Sprintf("tm_%d", tmTableSeq)
				if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (1000000, NULL)"); err != nil {
					t.Fatalf("insert NULL into %s: %v", colType, err)
				}
				n, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM "+tbl+" WHERE v IS NULL")
				if err != nil || len(n) != 1 || n[0] != 1 {
					t.Fatalf("NULL in %s: count = %v, %v", colType, n, err)
				}
			})
		}
		for k := range cases {
			if _, isDoc := doc[k]; !isDoc || doc[k] == "unsupported" {
				continue // the PostgreSQL and SQLite rows of the shared cases
			}
			if !covered[k] {
				t.Errorf("round-trip case %q has no documented MySQL row", k)
			}
		}
	})
}

// TestTypeMappingMySQLNull reads NULL from nullable columns of the documented
// MySQL types into pointers, and refuses it for a plain value.
func TestTypeMappingMySQLNull(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, `CREATE TABLE tm_nulls (id int PRIMARY KEY, i bigint, s text, t datetime(6),
			b blob, j json, u char(36), d decimal(10,2), f double, x text, dt text)`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, "INSERT INTO tm_nulls (id) VALUES (1)"); err != nil {
			t.Fatal(err)
		}
		check := func(name string, got any, err error, isNil bool) {
			t.Helper()
			if err != nil || !isNil {
				t.Errorf("%s: NULL read back as %v, %v", name, got, err)
			}
		}
		i, err := momo.Scalars[*int64](ctx, db, "SELECT i FROM tm_nulls")
		check("*int64", i, err, err == nil && len(i) == 1 && i[0] == nil)
		s, err := momo.Scalars[*string](ctx, db, "SELECT s FROM tm_nulls")
		check("*string", s, err, err == nil && len(s) == 1 && s[0] == nil)
		ts, err := momo.Scalars[*time.Time](ctx, db, "SELECT t FROM tm_nulls")
		check("*time.Time", ts, err, err == nil && len(ts) == 1 && ts[0] == nil)
		b, err := momo.Scalars[[]byte](ctx, db, "SELECT b FROM tm_nulls")
		check("[]byte", b, err, err == nil && len(b) == 1 && b[0] == nil)
		j, err := momo.Scalars[*types.JSON[tmDoc]](ctx, db, "SELECT j FROM tm_nulls")
		check("*types.JSON", j, err, err == nil && len(j) == 1 && j[0] == nil)
		u, err := momo.Scalars[*types.UUID](ctx, db, "SELECT u FROM tm_nulls")
		check("*types.UUID", u, err, err == nil && len(u) == 1 && u[0] == nil)
		d, err := momo.Scalars[*types.Numeric](ctx, db, "SELECT d FROM tm_nulls")
		check("*types.Numeric", d, err, err == nil && len(d) == 1 && d[0] == nil)
		x, err := momo.Scalars[*types.Numeric](ctx, db, "SELECT x FROM tm_nulls")
		check("*types.Numeric (text)", x, err, err == nil && len(x) == 1 && x[0] == nil)
		dt, err := momo.Scalars[*types.Date](ctx, db, "SELECT dt FROM tm_nulls")
		check("*types.Date", dt, err, err == nil && len(dt) == 1 && dt[0] == nil)
		f, err := momo.Scalars[*float64](ctx, db, "SELECT f FROM tm_nulls")
		check("*float64", f, err, err == nil && len(f) == 1 && f[0] == nil)
		if _, err := momo.Scalars[int64](ctx, db, "SELECT i FROM tm_nulls"); err == nil {
			t.Error("scanning NULL into int64 succeeded; want an error")
		}
	})
}

// TestTypeMappingMySQLUnsigned round-trips the unsigned column types the
// document names for the unsigned Go integers.
func TestTypeMappingMySQLUnsigned(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		run := func(goType string, want string, body func(tbl, col string) error) {
			t.Run(goType, func(t *testing.T) {
				col, err := typemap.Map(goType, dialect.MySQL, tags.Tags{})
				if err != nil || col != want {
					t.Fatalf("Map(%s) = %q, %v; document says %q", goType, col, err, want)
				}
				tmTableSeq++
				tbl := fmt.Sprintf("tm_%d", tmTableSeq)
				if _, err := momo.Exec(ctx, db, "CREATE TABLE "+tbl+" (id int PRIMARY KEY, v "+col+")"); err != nil {
					t.Fatalf("create: %v", err)
				}
				if err := body(tbl, col); err != nil {
					t.Fatal(err)
				}
			})
		}
		round := func(tbl string, vals ...uint64) error {
			for i, v := range vals {
				if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (?, ?)", i, v); err != nil {
					return fmt.Errorf("insert %d: %w", v, err)
				}
			}
			got, err := momo.Scalars[uint64](ctx, db, "SELECT v FROM "+tbl+" ORDER BY id")
			if err != nil || len(got) != len(vals) {
				return fmt.Errorf("select: %v, %v", got, err)
			}
			for i := range vals {
				if got[i] != vals[i] {
					return fmt.Errorf("stored %d, read back %d", vals[i], got[i])
				}
			}
			return nil
		}
		// The column type of each unsigned Go integer is read from the sentence
		// of the document that names them.
		docTypes := parseDocUnsigned(t)
		maxes := map[string]uint64{
			"uint8": math.MaxUint8, "uint16": math.MaxUint16, "uint32": math.MaxUint32,
			"uint": math.MaxUint64, "uint64": math.MaxUint64,
		}
		for goType, max := range maxes {
			want, ok := docTypes[goType]
			if !ok {
				t.Errorf("%s does not name a column type for %s", typeMappingDoc, goType)
				continue
			}
			run(goType, want, func(tbl, _ string) error { return round(tbl, 0, max) })
		}
		for goType := range docTypes {
			if _, ok := maxes[goType]; !ok {
				t.Errorf("%s names an unsigned type %s that has no round-trip case", typeMappingDoc, goType)
			}
		}
		t.Run("out of range", func(t *testing.T) {
			if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_u8 (v tinyint unsigned)"); err != nil {
				t.Fatal(err)
			}
			if _, err := momo.Exec(ctx, db, "INSERT INTO tm_u8 (v) VALUES (?)", 256); err == nil {
				t.Error("256 into tinyint unsigned succeeded, want an error")
			}
			if _, err := momo.Exec(ctx, db, "INSERT INTO tm_u8 (v) VALUES (?)", -1); err == nil {
				t.Error("-1 into tinyint unsigned succeeded, want an error")
			}
		})
	})
}

// TestTypeMappingMySQLLimits pins the documented MySQL limits: text and blob
// hold 64 KiB, a varchar refuses a longer string, decimal(p,s) keeps its
// precision up to 65 digits, and the values a MySQL column cannot hold are
// refused rather than stored.
func TestTypeMappingMySQLLimits(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_lim (t text, b blob, v varchar(3), f double, r float)"); err != nil {
			t.Fatal(err)
		}
		for name, c := range map[string]struct {
			col string
			arg any
		}{
			"text over 64 KiB":  {"t", strings.Repeat("x", 65536)},
			"blob over 64 KiB":  {"b", bytes.Repeat([]byte{1}, 65536)},
			"varchar over size": {"v", "abcd"},
			"double NaN":        {"f", math.NaN()},
			"double +Inf":       {"f", math.Inf(1)},
			"float NaN":         {"r", float32(math.NaN())},
			"float above max":   {"r", math.MaxFloat64},
		} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO tm_lim ("+c.col+") VALUES (?)", c.arg); err == nil {
				t.Errorf("%s: insert succeeded, want an error", name)
			}
		}

		// FLOAT reads back with six significant digits: the largest float32
		// stays within 1e-5 of what was stored but is not bit-identical.
		if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_f32 (v float)"); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, "INSERT INTO tm_f32 (v) VALUES (?)", float32(math.MaxFloat32)); err != nil {
			t.Fatal(err)
		}
		for name, q := range map[string]struct {
			sql  string
			args []any
		}{
			"binary protocol (query with an argument)": {"SELECT v FROM tm_f32 WHERE ? = 1", []any{1}},
			"CAST AS DOUBLE": {"SELECT CAST(v AS DOUBLE) FROM tm_f32", nil},
			"v + 0e0":        {"SELECT v + 0e0 FROM tm_f32", nil},
		} {
			got, err := momo.Scalars[float32](ctx, db, q.sql, q.args...)
			if err != nil || len(got) != 1 || got[0] != math.MaxFloat32 {
				t.Errorf("%s: MaxFloat32 read back %v, %v", name, got, err)
			}
		}
		// An inlined float32 literal must fit the FLOAT column: the shortest
		// text of the largest float32, 3.4028235e+38, is out of range (error
		// 1264) on every server, so Lit writes it with one digit fewer.
		if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_flit (v float)"); err != nil {
			t.Fatal(err)
		}
		for _, x := range []float32{math.MaxFloat32, -math.MaxFloat32} {
			c, err := exprtest.CompileColumns(dialect.MySQL, momo.Lit(x))
			if err != nil {
				t.Fatal(err)
			}
			lit, _, _ := strings.Cut(strings.TrimPrefix(c.SQL, "SELECT "), " FROM ")
			if _, err := momo.Exec(ctx, db, "INSERT INTO tm_flit (v) VALUES ("+lit+")"); err != nil {
				t.Errorf("literal %s for %v into FLOAT: %v", lit, x, err)
			}
		}
		if _, err := momo.Exec(ctx, db, "INSERT INTO tm_flit (v) VALUES (3.4028235e+38)"); err == nil {
			t.Error("the shortest text of MaxFloat32 was taken by a FLOAT column; the literal form in Lit has no reason to differ")
		}
		// A query with no argument comes back over the text protocol, where the
		// server prints a FLOAT with six significant digits.
		plain, err := momo.Scalars[float32](ctx, db, "SELECT v FROM tm_f32")
		if err != nil || len(plain) != 1 || math.Abs(float64(plain[0])-math.MaxFloat32)/math.MaxFloat32 > 1e-5 {
			t.Errorf("text protocol: MaxFloat32 read back %v, %v; want six digits", plain, err)
		}

		// decimal(p,s): the generator's type for a precision tag keeps every digit.
		dec, err := typemap.Map("types.Numeric", dialect.MySQL, tags.Tags{Precision: 65, Scale: 30})
		if err != nil || dec != "decimal(65,30)" {
			t.Fatalf("Map(types.Numeric, 65, 30) = %q, %v", dec, err)
		}
		nines := strings.Repeat("9", 35) + "." + strings.Repeat("9", 30)
		tiny := "0." + strings.Repeat("0", 29) + "1"
		roundTripAll[types.Numeric](nil,
			types.Numeric{Decimal: nines}, types.Numeric{Decimal: "-" + nines}, types.Numeric{Decimal: tiny})(t, db, dec)
		scale0, err := typemap.Map("types.Numeric", dialect.MySQL, tags.Tags{Precision: 10})
		if err != nil || scale0 != "decimal(10)" {
			t.Fatalf("Map(types.Numeric, 10) = %q, %v", scale0, err)
		}
		roundTripAll[types.Numeric](nil, types.Numeric{Decimal: "9999999999"}, types.Numeric{Decimal: "-9999999999"})(t, db, scale0)
		if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_dec (v decimal(10,2))"); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, "INSERT INTO tm_dec (v) VALUES (?)", types.Numeric{Decimal: "100000000.00"}); err == nil {
			t.Error("a 11-digit value into decimal(10,2) succeeded, want an error")
		}

		// NaN, infinity and special dates cannot be stored on MySQL.
		if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_ref (n text, d text)"); err != nil {
			t.Fatal(err)
		}
		for name, v := range map[string]types.Numeric{
			"NaN": {NaN: true}, "Infinity": {Infinity: true}, "NegInfinity": {NegInfinity: true},
		} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO tm_ref (n) VALUES (?)", v); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("types.Numeric %s: err = %v, want ErrValidation", name, err)
			}
		}
		for name, v := range map[string]types.Date{
			"Infinity": {Infinity: true}, "NegInfinity": {NegInfinity: true},
		} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO tm_ref (d) VALUES (?)", v); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("types.Date %s: err = %v, want ErrValidation", name, err)
			}
		}
		if n, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM tm_ref"); err != nil || len(n) != 1 || n[0] != 0 {
			t.Errorf("rows stored after refused values = %v, %v", n, err)
		}
	})
}
