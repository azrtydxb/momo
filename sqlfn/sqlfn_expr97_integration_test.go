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
)

func createExpr97(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS expr97",
		"CREATE TABLE expr97 (id bigint primary key, at timestamptz not null, day date not null, marks double precision not null, max_marks double precision not null, grp bigint)",
		"INSERT INTO expr97 VALUES (1, '2026-01-01T00:00:00Z', CURRENT_DATE - 1, 50, 200, 1)",
		"INSERT INTO expr97 VALUES (2, '2026-01-01T00:00:00Z', CURRENT_DATE, 10, 0, NULL)",
		"INSERT INTO expr97 VALUES (3, '2026-01-02T00:00:00Z', CURRENT_DATE + 1, 1, 100, NULL)",
		"INSERT INTO expr97 VALUES (4, 'infinity', CURRENT_DATE + 2, 1, 100, 1)",
		"INSERT INTO expr97 VALUES (5, '-infinity', CURRENT_DATE - 2, 1, 100, 1)",
	} {
		if _, err := db.Driver().Exec(ctx, s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func count97(t *testing.T, db *momo.DB, p momo.Predicate) int64 {
	t.Helper()
	r, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "expr97"}).
		Columns(momo.CountAll()).Where(p).One(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return r.N
}

// Catches: row-value comparison with the wrong lexicographic meaning (a
// column-wise AND), or SQL the server rejects.
func TestRowComparisonExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createExpr97(t, db)
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		pos := func() momo.RowExpr { return momo.Row(momo.Value(at), momo.Value(int64(2))) }
		row := momo.Row(e97At, e97ID)
		// Rows before (2026-01-01, 2): id 5 (-infinity) and id 1 (same time, id 1).
		if n := count97(t, db, row.Lt(pos())); n != 2 {
			t.Errorf("Lt = %d, want 2", n)
		}
		// Plus id 2 itself.
		if n := count97(t, db, row.Le(pos())); n != 3 {
			t.Errorf("Le = %d, want 3", n)
		}
		// After: id 3 and id 4 (infinity).
		if n := count97(t, db, row.Gt(pos())); n != 2 {
			t.Errorf("Gt = %d, want 2", n)
		}
		if n := count97(t, db, row.Ge(pos())); n != 3 {
			t.Errorf("Ge = %d, want 3", n)
		}
	})
}

// Catches: division by a zero divisor erroring (NULLIF missing) or dividing
// as integers, and the wrong rows passing.
func TestDivNullIfExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createExpr97(t, db)
		ratio := sqlfn.DivExpr[float64](e97Marks, sqlfn.NullIf[float64](e97Max, 0))
		// 50/200 = 0.25 passes; 10/0 is NULL; 1/100 = 0.01 fails.
		if n := count97(t, db, ratio.Gt(0.2)); n != 1 {
			t.Errorf("ratio > 0.2 = %d, want 1", n)
		}
		if n := count97(t, db, sqlfn.Div[float64](e97Marks, 4).Gt(10)); n != 1 {
			t.Errorf("marks / 4 > 10 = %d, want 1", n)
		}
	})
}

// Catches: CURRENT_DATE and the infinity literals failing on PostgreSQL.
func TestCurrentDateAndInfinityExecute(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createExpr97(t, db)
		if n := count97(t, db, e97Day.LtExpr(sqlfn.CurrentDate())); n != 2 {
			t.Errorf("day < CURRENT_DATE = %d, want 2", n)
		}
		// Finite timestamps: ids 1, 2, 3.
		if n := count97(t, db, momo.And(e97At.LtExpr(sqlfn.Infinity()), e97At.GtExpr(sqlfn.NegInfinity()))); n != 3 {
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
