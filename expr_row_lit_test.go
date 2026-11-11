// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
)

var (
	rowAt = momo.NewColumn[time.Time]("waiters", "created_at", 0)
	rowID = momo.NewColumn[int64]("waiters", "id", 0)
)

// Catches: a row comparison rendered with the wrong operator, unparenthesised,
// or with its values interpolated instead of bound, on any dialect.
func TestRowComparisonGolden(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name string
		mk   func(l, r momo.RowExpr) momo.Predicate
		op   string
	}{
		{"lt", momo.RowExpr.Lt, "<"},
		{"le", momo.RowExpr.Le, "<="},
		{"gt", momo.RowExpr.Gt, ">"},
		{"ge", momo.RowExpr.Ge, ">="},
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		q := func(s string) string {
			if d == dialect.MySQL {
				return "`" + s + "`"
			}
			return `"` + s + `"`
		}
		ph1, ph2 := "$1", "$2"
		if d != dialect.Postgres {
			ph1, ph2 = "?", "?"
		}
		for _, c := range cases {
			p := c.mk(momo.Row(rowAt, rowID), momo.Row(momo.Value(ts), momo.Value(int64(7))))
			got, err := exprtest.Compile(d, p)
			if err != nil {
				t.Fatalf("%s %s: %v", d, c.name, err)
			}
			want := "SELECT COUNT(*) FROM " + q("docs") + " WHERE (" + q("waiters") + "." + q("created_at") + ", " + q("waiters") + "." + q("id") + ") " + c.op + " (" + ph1 + ", " + ph2 + ")"
			if got.SQL != want || len(got.Args) != 2 {
				t.Errorf("%s %s:\n got %s %v\nwant %s", d, c.name, got.SQL, got.Args, want)
			}
		}
	}
}

// Catches: a row comparison with mismatched or too-short rows reaching the
// database instead of failing the build with ErrValidation.
func TestRowComparisonValidation(t *testing.T) {
	for name, p := range map[string]momo.Predicate{
		"length mismatch": momo.Row(rowAt, rowID).Lt(momo.Row(momo.Value(int64(1)))),
		"single element":  momo.Row(rowID).Lt(momo.Row(momo.Value(int64(1)))),
		"empty":           momo.Row().Gt(momo.Row()),
	} {
		_, err := exprtest.Compile(dialect.Postgres, p)
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}

// Catches: Lit binding a parameter (so SELECT and GROUP BY texts differ), a
// negative number fusing with an operator, a float losing its decimal point,
// bool in the wrong spelling per dialect, and a date keeping a time of day.
func TestLitGolden(t *testing.T) {
	type tc struct {
		name string
		e    momo.Expr
		pg   string
		sl   string
	}
	day := time.Date(2026, 3, 4, 23, 59, 0, 0, time.UTC)
	for _, c := range []tc{
		{"int", momo.Lit(42), "42", "42"},
		{"negative", momo.Lit(int64(-5)), "(-5)", "(-5)"},
		{"uint", momo.Lit(uint8(7)), "7", "7"},
		{"float whole", momo.Lit(2.0), "2.0", "2.0"},
		{"float frac", momo.Lit(0.25), "0.25", "0.25"},
		{"float negative", momo.Lit(-1.5), "(-1.5)", "(-1.5)"},
		{"bool true", momo.Lit(true), "TRUE", "1"},
		{"bool false", momo.Lit(false), "FALSE", "0"},
		{"date", momo.Lit(day), "DATE '2026-03-04'", "'2026-03-04'"},
	} {
		for d, want := range map[dialect.Name]string{dialect.Postgres: c.pg, dialect.SQLite: c.sl} {
			got, err := exprtest.CompileColumns(d, c.e)
			if err != nil {
				t.Fatalf("%s %s: %v", d, c.name, err)
			}
			w := `SELECT ` + want + ` FROM "documents"`
			if got.SQL != w || len(got.Args) != 0 {
				t.Errorf("%s %s:\n got %s %v\nwant %s", d, c.name, got.SQL, got.Args, w)
			}
		}
	}
}

// Catches: a non-finite float rendering as NaN or Inf text instead of failing
// the build.
func TestLitRejectsNonFinite(t *testing.T) {
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := exprtest.CompileColumns(dialect.Postgres, momo.Lit(f)); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("Lit(%v): err = %v, want ErrValidation", f, err)
		}
	}
}

// Catches: the same Lit in SELECT and GROUP BY rendering different text (the
// 42803 failure of bound parameters) or carrying bound arguments.
func TestLitSelectAndGroupByIdentical(t *testing.T) {
	k := momo.NewColumn[int64]("documents", "k", 0)
	e := func() momo.Expr { return momo.Func[int64]("coalesce", k, momo.Lit(int64(0))) }
	got, err := momo.Select[exprtest.Pair](exprtest.DB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(e(), momo.CountAll()).GroupBy(e()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT coalesce("documents"."k", 0), COUNT(*) FROM "documents" GROUP BY coalesce("documents"."k", 0)`
	if got.SQL != want || len(got.Args) != 0 {
		t.Errorf("got %s %v\nwant %s", got.SQL, got.Args, want)
	}
}
