// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlfn_test

import (
	"context"
	"math"
	"strconv"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
)

// eachAllDB runs fn on PostgreSQL, SQLite, MySQL 8.4, MySQL 9.7 and MariaDB
// 11.8, one subtest each. testopen.Each covers only the first two, so the MySQL
// servers are opened by name; a server whose URL is unset is skipped, and
// failed under MOMO_REQUIRE_INTEGRATION=1 for the dialects the job requires.
func eachAllDB(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name)) {
	t.Helper()
	for _, s := range []struct {
		name string
		d    dialect.Name
		open func(*testing.T, ...momo.Option) *momo.DB
	}{
		{"postgres", dialect.Postgres, testopen.Postgres},
		{"sqlite", dialect.SQLite, testopen.SQLite},
		{"mysql84", dialect.MySQL, testopen.MySQL84},
		{"mysql97", dialect.MySQL, testopen.MySQL97},
		{"mariadb118", dialect.MySQL, testopen.MariaDB},
	} {
		t.Run(s.name, func(t *testing.T) {
			fn(t, s.open(t), s.d)
		})
	}
}

var (
	xiID = momo.NewColumn[int64]("xt_ext", "id", 0)
	xiA  = momo.NewNullColumn[int64]("xt_ext", "a", 0)
	xiB  = momo.NewNullColumn[int64]("xt_ext", "b", 0)
	xiC  = momo.NewNullColumn[int64]("xt_ext", "c", 0)
	xiS  = momo.NewNullColumn[string]("xt_ext", "s", 0)
	xiT  = momo.NewNullColumn[string]("xt_ext", "t", 0)
	xiF  = momo.NewNullColumn[float64]("xt_ext", "f", 0)
)

type xtInt struct {
	ID int64
	V  *int64
}

var xtIntMeta = momo.Meta[xtInt]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "v"}},
	Ptr:      func(m *xtInt, i int) any { return [...]any{&m.ID, &m.V}[i] },
	Value:    func(m *xtInt, i int) any { return [...]any{m.ID, m.V}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*xtInt) MomoMeta() *momo.Meta[xtInt] { return &xtIntMeta }

type xtStr struct {
	ID int64
	V  *string
}

var xtStrMeta = momo.Meta[xtStr]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "v"}},
	Ptr:      func(m *xtStr, i int) any { return [...]any{&m.ID, &m.V}[i] },
	Value:    func(m *xtStr, i int) any { return [...]any{m.ID, m.V}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*xtStr) MomoMeta() *momo.Meta[xtStr] { return &xtStrMeta }

type xtFloat struct {
	ID int64
	V  *float64
}

var xtFloatMeta = momo.Meta[xtFloat]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "v"}},
	Ptr:      func(m *xtFloat, i int) any { return [...]any{&m.ID, &m.V}[i] },
	Value:    func(m *xtFloat, i int) any { return [...]any{m.ID, m.V}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*xtFloat) MomoMeta() *momo.Meta[xtFloat] { return &xtFloatMeta }

// seedExt creates xt_ext. Rows (a, b, c | s, t | f):
//
//	1: 1 5 3    | apple pear | 2.5
//	2: - 5 3    | -     pear | -2.5
//	3: - - -    | -     -    | -
//	4: 7 - -    | zed   -    | 3.0
//	5: 0 -4 -   | kiwi  kiwi | -0.5
//	6: -2 -8 -9 | -     -    | 1e19
func seedExt(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	text, float := "text", "double precision"
	switch d {
	case dialect.MySQL:
		text, float = "varchar(40)", "double"
	case dialect.SQLite:
		float = "real"
	}
	exec97(t, db,
		"DROP TABLE IF EXISTS xt_ext",
		"CREATE TABLE xt_ext (id bigint primary key, a bigint, b bigint, c bigint, s "+text+", t "+text+", f "+float+")",
		"INSERT INTO xt_ext VALUES (1, 1, 5, 3, 'apple', 'pear', 2.5)",
		"INSERT INTO xt_ext VALUES (2, NULL, 5, 3, NULL, 'pear', -2.5)",
		"INSERT INTO xt_ext VALUES (3, NULL, NULL, NULL, NULL, NULL, NULL)",
		"INSERT INTO xt_ext VALUES (4, 7, NULL, NULL, 'zed', NULL, 3.0)",
		"INSERT INTO xt_ext VALUES (5, 0, -4, NULL, 'kiwi', 'kiwi', -0.5)",
		"INSERT INTO xt_ext VALUES (6, -2, -8, -9, NULL, NULL, 1e19)",
	)
}

func ptr[T any](v T) *T { return &v }

func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func fmtPtr[T any](p *T) string {
	if p == nil {
		return "NULL"
	}
	switch v := any(*p).(type) {
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		return v
	}
	return "?"
}

func wantInts(t *testing.T, label string, db *momo.DB, e momo.Expr, want []*int64) {
	t.Helper()
	rows, err := momo.Select[xtInt](db).From(momo.TableRef{Name: "xt_ext"}).Columns(xiID, e).
		OrderBy(momo.Asc(xiID)).All(context.Background())
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if len(rows) != len(want) {
		t.Errorf("%s: %d rows, want %d", label, len(rows), len(want))
		return
	}
	for i, r := range rows {
		if !samePtr(r.V, want[i]) {
			t.Errorf("%s: row %d = %s, want %s", label, r.ID, fmtPtr(r.V), fmtPtr(want[i]))
		}
	}
}

func wantStrs(t *testing.T, label string, db *momo.DB, e momo.Expr, want []*string) {
	t.Helper()
	rows, err := momo.Select[xtStr](db).From(momo.TableRef{Name: "xt_ext"}).Columns(xiID, e).
		OrderBy(momo.Asc(xiID)).All(context.Background())
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if len(rows) != len(want) {
		t.Errorf("%s: %d rows, want %d", label, len(rows), len(want))
		return
	}
	for i, r := range rows {
		if !samePtr(r.V, want[i]) {
			t.Errorf("%s: row %d = %s, want %s", label, r.ID, fmtPtr(r.V), fmtPtr(want[i]))
		}
	}
}

func wantFloats(t *testing.T, label string, db *momo.DB, e momo.Expr, want []*float64) {
	t.Helper()
	rows, err := momo.Select[xtFloat](db).From(momo.TableRef{Name: "xt_ext"}).Columns(xiID, e).
		OrderBy(momo.Asc(xiID)).All(context.Background())
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if len(rows) != len(want) {
		t.Errorf("%s: %d rows, want %d", label, len(rows), len(want))
		return
	}
	for i, r := range rows {
		if !samePtr(r.V, want[i]) {
			t.Errorf("%s: row %d = %s, want %s", label, r.ID, fmtPtr(r.V), fmtPtr(want[i]))
		}
	}
}

// Catches: GREATEST or LEAST returning NULL when one argument is NULL (the
// MySQL and SQLite rule) instead of ignoring it as PostgreSQL does, an
// emulation that drops or duplicates an argument, and a bound or literal
// operand that the rewrite mishandles. Runs on PostgreSQL, SQLite, MySQL 8.4,
// MySQL 9.7 and MariaDB 11.8.
func TestGreatestLeastExecute(t *testing.T) {
	eachAllDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedExt(t, db, d)
		n := func(v int64) *int64 { return ptr(v) }
		wantInts(t, "Greatest(a,b,c)", db, sqlfn.Greatest[int64](xiA, xiB, xiC),
			[]*int64{n(5), n(5), nil, n(7), n(0), n(-2)})
		wantInts(t, "Least(a,b,c)", db, sqlfn.Least[int64](xiA, xiB, xiC),
			[]*int64{n(1), n(3), nil, n(7), n(-4), n(-9)})
		// Two arguments, one of them a literal: the floor of one, the shape of
		// GREATEST(1, x).
		wantInts(t, "Greatest(1,a)", db, sqlfn.Greatest[int64](momo.Lit(int64(1)), xiA),
			[]*int64{n(1), n(1), n(1), n(7), n(1), n(1)})
		wantInts(t, "Least(1,a)", db, sqlfn.Least[int64](momo.Lit(int64(1)), xiA),
			[]*int64{n(1), n(1), n(1), n(1), n(0), n(-2)})
		// A bound value argument repeats once per rewrite operand.
		wantInts(t, "Greatest(Value 4,a,b)", db, sqlfn.Greatest[int64](momo.Value(int64(4)), xiA, xiB),
			[]*int64{n(5), n(5), n(4), n(7), n(4), n(4)})
		s := func(v string) *string { return ptr(v) }
		wantStrs(t, "Greatest(s,t)", db, sqlfn.Greatest[string](xiS, xiT),
			[]*string{s("pear"), s("pear"), nil, s("zed"), s("kiwi"), nil})
		wantStrs(t, "Least(s,t)", db, sqlfn.Least[string](xiS, xiT),
			[]*string{s("apple"), s("pear"), nil, s("zed"), s("kiwi"), nil})
		// The result composes: it is an Expression with comparison methods.
		if n := count97(t, db, "xt_ext", sqlfn.Greatest[int64](xiA, xiB).Eq(5)); n != 2 {
			t.Errorf("Greatest(a,b) = 5 matched %d rows, want 2", n)
		}
	})
}

// Catches: Floor or Ceil rounding toward zero, returning an integer type that
// does not scan as a float, losing NULL, failing on SQLite builds without the
// math functions, or mangling a float too large for an INTEGER cast. Runs on
// PostgreSQL, SQLite, MySQL 8.4, MySQL 9.7 and MariaDB 11.8.
func TestFloorCeilExecute(t *testing.T) {
	eachAllDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedExt(t, db, d)
		f := func(v float64) *float64 { return ptr(v) }
		wantFloats(t, "Floor(f)", db, sqlfn.Floor[float64](xiF),
			[]*float64{f(2), f(-3), nil, f(3), f(-1), f(1e19)})
		wantFloats(t, "Ceil(f)", db, sqlfn.Ceil[float64](xiF),
			[]*float64{f(3), f(-2), nil, f(3), f(0), f(1e19)})
		if n := count97(t, db, "xt_ext", sqlfn.Floor[float64](xiF).Eq(-3)); n != 1 {
			t.Errorf("Floor(f) = -3 matched %d rows, want 1", n)
		}
		if n := count97(t, db, "xt_ext", sqlfn.Ceil[float64](xiF).Eq(-2)); n != 1 {
			t.Errorf("Ceil(f) = -2 matched %d rows, want 1", n)
		}
	})
}

type xtNum struct {
	ID int64
	N  types.Numeric
}

var xtNumMeta = momo.Meta[xtNum]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "n"}},
	Ptr:      func(m *xtNum, i int) any { return [...]any{&m.ID, &m.N}[i] },
	Value:    func(m *xtNum, i int) any { return [...]any{m.ID, m.N}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*xtNum) MomoMeta() *momo.Meta[xtNum] { return &xtNumMeta }

// Catches: Floor, Ceil, Greatest or Least over a numeric column failing on
// PostgreSQL, or going through float8 and losing the Numeric result type.
func TestNumericExtremesAndRoundingOnPostgres(t *testing.T) {
	db := testopen.Postgres(t)
	exec97(t, db,
		"DROP TABLE IF EXISTS xt_num",
		"CREATE TABLE xt_num (id bigint primary key, n numeric(12,6) not null)",
		"INSERT INTO xt_num VALUES (1, 2.7)",
		"INSERT INTO xt_num VALUES (2, -2.7)",
	)
	id := momo.NewColumn[int64]("xt_num", "id", 0)
	n := momo.NewColumn[types.Numeric]("xt_num", "n", 0)
	for _, c := range []struct {
		name string
		e    momo.Expr
		want [2]float64
	}{
		{"Floor", sqlfn.Floor[types.Numeric](n), [2]float64{2, -3}},
		{"Ceil", sqlfn.Ceil[types.Numeric](n), [2]float64{3, -2}},
		{"Greatest", sqlfn.Greatest[types.Numeric](n, momo.Value(types.Numeric{Decimal: "0"})), [2]float64{2.7, 0}},
		{"Least", sqlfn.Least[types.Numeric](n, momo.Value(types.Numeric{Decimal: "0"})), [2]float64{0, -2.7}},
	} {
		rows, err := momo.Select[xtNum](db).From(momo.TableRef{Name: "xt_num"}).Columns(id, c.e).
			OrderBy(momo.Asc(id)).All(context.Background())
		if err != nil || len(rows) != 2 {
			t.Errorf("%s: %v rows, %v", c.name, len(rows), err)
			continue
		}
		for i, r := range rows {
			got, perr := strconv.ParseFloat(r.N.Decimal, 64)
			if perr != nil || math.Abs(got-c.want[i]) > 1e-9 {
				t.Errorf("%s row %d = %q, want %v", c.name, r.ID, r.N.Decimal, c.want[i])
			}
		}
	}
}
