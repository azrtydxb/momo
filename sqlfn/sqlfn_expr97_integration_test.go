// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlfn_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
)

var (
	e97ID    = momo.NewColumn[int64]("expr97", "id", 0)
	e97At    = momo.NewColumn[time.Time]("expr97", "at", 0)
	e97Day   = momo.NewColumn[time.Time]("expr97", "day", 0)
	e97Marks = momo.NewColumn[float64]("expr97", "marks", 0)
	e97Max   = momo.NewColumn[float64]("expr97", "max_marks", 0)
	e97Grp   = momo.NewColumn[int64]("expr97", "grp", 0)
	e97Tag   = momo.NewColumn[string]("expr97", "tag", 0)

	rowA  = momo.NewColumn[int64]("expr97r", "a", 0)
	rowID = momo.NewColumn[int64]("expr97r", "id", 0)

	dateDay = momo.NewColumn[time.Time]("expr97d", "day", 0)
)

func exec97(t *testing.T, db *momo.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Driver().Exec(context.Background(), s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// createExpr97 is the PostgreSQL table of the expression tests.
func createExpr97(t *testing.T, db *momo.DB) {
	t.Helper()
	exec97(t, db,
		"DROP TABLE IF EXISTS expr97",
		"CREATE TABLE expr97 (id bigint primary key, at timestamptz not null, day date not null, marks double precision not null, max_marks double precision not null, grp bigint, tag text)",
		"INSERT INTO expr97 VALUES (1, '2026-01-01T00:00:00Z', '2026-03-04', 50, 200, 1, 'a')",
		"INSERT INTO expr97 VALUES (2, '2026-01-01T00:00:00Z', '2026-03-05', 10, 0, NULL, NULL)",
		"INSERT INTO expr97 VALUES (3, '2026-01-02T00:00:00Z', '2026-03-06', 1, 100, NULL, NULL)",
		"INSERT INTO expr97 VALUES (4, 'infinity', '2026-03-07', 1, 100, 1, 'a')",
		"INSERT INTO expr97 VALUES (5, '-infinity', '2026-03-08', 1, 100, 1, NULL)",
	)
}

func count97(t *testing.T, db *momo.DB, table string, p momo.Predicate) int64 {
	t.Helper()
	r, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: table}).
		Columns(momo.CountAll()).Where(p).One(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return r.N
}

// Catches: row-value comparison with the wrong lexicographic meaning (a
// column-wise AND), or SQL a dialect rejects. Runs on every dialect.
func TestRowComparisonExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		exec97(t, db,
			"DROP TABLE IF EXISTS expr97r",
			"CREATE TABLE expr97r (id bigint primary key, a bigint not null)",
			"INSERT INTO expr97r VALUES (1, 1)",
			"INSERT INTO expr97r VALUES (2, 1)",
			"INSERT INTO expr97r VALUES (3, 2)",
			"INSERT INTO expr97r VALUES (4, 2)",
			"INSERT INTO expr97r VALUES (5, 3)",
		)
		pos := func() momo.RowExpr { return momo.Row(momo.Value(int64(2)), momo.Value(int64(3))) }
		row := momo.Row(rowA, rowID)
		// Rows are (a, id): (1,1) (1,2) (2,3) (2,4) (3,5); the position is (2, 3).
		for name, c := range map[string]struct {
			p    momo.Predicate
			want int64
		}{
			"Lt": {row.Lt(pos()), 2},
			"Le": {row.Le(pos()), 3},
			"Gt": {row.Gt(pos()), 2},
			"Ge": {row.Ge(pos()), 3},
		} {
			if n := count97(t, db, "expr97r", c.p); n != c.want {
				t.Errorf("%s: %s = %d, want %d", d, name, n, c.want)
			}
		}
	})
}

// Catches: row-value comparison over timestamps and a uuid-like key failing
// on PostgreSQL, the FIFO position check of the issue.
func TestRowComparisonTimestampExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("timestamptz and infinity are PostgreSQL")
		}
		createExpr97(t, db)
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		pos := momo.Row(momo.Value(at), momo.Value(int64(2)))
		row := momo.Row(e97At, e97ID)
		// Before (2026-01-01, 2): id 5 (-infinity) and id 1.
		if n := count97(t, db, "expr97", row.Lt(pos)); n != 2 {
			t.Errorf("Lt = %d, want 2", n)
		}
		if n := count97(t, db, "expr97", row.Ge(pos)); n != 3 {
			t.Errorf("Ge = %d, want 3", n)
		}
	})
}

// Catches: division by a zero divisor erroring (NULLIF missing), integer
// division returning a fraction, and the wrong rows passing.
func TestDivNullIfExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration; MySQL integer division returns DECIMAL")
		}
		createExpr97(t, db)
		ratio := sqlfn.DivExpr[float64](e97Marks, sqlfn.NullIf[float64](e97Max, 0))
		// 50/200 = 0.25 passes; 10/0 is NULL; 1/100 = 0.01 fails.
		if n := count97(t, db, "expr97", ratio.Gt(0.2)); n != 1 {
			t.Errorf("ratio > 0.2 = %d, want 1", n)
		}
		if n := count97(t, db, "expr97", sqlfn.Div[float64](e97Marks, 4).Gt(10)); n != 1 {
			t.Errorf("marks / 4 > 10 = %d, want 1", n)
		}
		// Integer division on bigint: ids 1..5 / 2 = 0, 1, 1, 2, 2, so 7 / 2 style
		// truncation puts ids 4 and 5 at 2; a fractional result would match none.
		if n := count97(t, db, "expr97", sqlfn.Div[int64](e97ID, 2).Eq(2)); n != 2 {
			t.Errorf("id / 2 = 2 gave %d rows, want 2 (integer division)", n)
		}
		// The same with a guarded integer divisor and a literal.
		if n := count97(t, db, "expr97", sqlfn.DivExpr[int64](momo.Lit(int64(7)), sqlfn.NullIf[int64](e97ID, 0)).Eq(3)); n != 1 {
			t.Errorf("7 / id = 3 gave %d rows, want 1 (id 2)", n)
		}
	})
}

// Catches: CURRENT_DATE failing or comparing wrongly on any dialect (it is
// text on SQLite).
func TestCurrentDateExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		exec97(t, db,
			"DROP TABLE IF EXISTS expr97d",
			"CREATE TABLE expr97d (id bigint primary key, day date not null)",
			"INSERT INTO expr97d VALUES (1, '2000-01-01')",
			"INSERT INTO expr97d VALUES (2, '2999-12-31')",
		)
		if n := count97(t, db, "expr97d", dateDay.LtExpr(sqlfn.CurrentDate())); n != 1 {
			t.Errorf("%s: day < CURRENT_DATE = %d, want 1", d, n)
		}
		if n := count97(t, db, "expr97d", dateDay.GtExpr(sqlfn.CurrentDate())); n != 1 {
			t.Errorf("%s: day > CURRENT_DATE = %d, want 1", d, n)
		}
	})
}

// Catches: the infinity literals failing on PostgreSQL.
func TestInfinityExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("infinity timestamps are PostgreSQL only")
		}
		createExpr97(t, db)
		// Finite timestamps: ids 1, 2, 3.
		if n := count97(t, db, "expr97", momo.And(e97At.LtExpr(sqlfn.Infinity()), e97At.GtExpr(sqlfn.NegInfinity()))); n != 3 {
			t.Errorf("finite = %d, want 3", n)
		}
	})
}

// Catches: a literal bound as a parameter, so SELECT and GROUP BY differ and
// PostgreSQL rejects the grouping (42803).
func TestLitGroupByExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createExpr97(t, db)
		key := func() momo.Expression[int64] { return sqlfn.CoalesceExpr[int64](e97Grp, momo.Lit(int64(0))) }
		rows, err := momo.Select[exprtest.Pair](db).From(momo.TableRef{Name: "expr97"}).
			Columns(key(), momo.CountAll()).GroupBy(key()).OrderBy(momo.Asc(key())).All(context.Background())
		if err != nil || len(rows) != 2 || rows[0].K != 0 || rows[0].N != 2 || rows[1].K != 1 || rows[1].N != 3 {
			t.Fatalf("group by coalesce literal = %+v, %v", rows, err)
		}
	})
}

// Catches: COALESCE(col, 'none') in SELECT and GROUP BY failing with 42803,
// the exact case of the issue, and a wrong string literal.
func TestLitStringGroupByExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createExpr97(t, db)
		key := func() momo.Expression[string] { return sqlfn.CoalesceExpr[string](e97Tag, momo.Lit("none")) }
		rows, err := momo.Select[strCount](db).From(momo.TableRef{Name: "expr97"}).
			Columns(key(), momo.CountAll()).GroupBy(key()).OrderBy(momo.Asc(key())).All(context.Background())
		if err != nil || len(rows) != 2 || rows[0].S != "a" || rows[0].N != 2 || rows[1].S != "none" || rows[1].N != 3 {
			t.Fatalf("group by coalesce string literal = %+v, %v", rows, err)
		}
		// A quote in the literal stays one string: it matches nothing and runs.
		if n := count97(t, db, "expr97", e97Tag.EqExpr(momo.Lit("a' OR '1'='1"))); n != 0 {
			t.Errorf("quoted literal matched %d rows, want 0", n)
		}
	})
}

// Catches: float, bool and date literals rendering text PostgreSQL rejects or
// reads as another value.
func TestLitFloatBoolDateExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createExpr97(t, db)
		day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
		p := momo.And(
			e97Marks.EqExpr(momo.Lit(50.0)),
			e97Day.EqExpr(momo.Lit(day)),
			momo.Lit(true).Eq(true),
			momo.Lit(false).Eq(false),
			momo.Lit(-1.5).Lt(0),
		)
		if n := count97(t, db, "expr97", p); n != 1 {
			t.Errorf("float, bool and date literals matched %d rows, want 1", n)
		}
	})
}
