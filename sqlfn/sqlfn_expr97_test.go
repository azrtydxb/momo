// SPDX-License-Identifier: Apache-2.0

package sqlfn_test

import (
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/sqlfn"
)

var (
	marksCol = momo.NewColumn[float64]("results", "final_marks", 0)
	maxCol   = momo.NewColumn[float64]("results", "max_marks", 0)
)

// Catches: division or NULLIF rendered wrongly, the NULLIF value bound (the
// text then differs between SELECT and GROUP BY), per dialect.
func TestDivAndNullIfGolden(t *testing.T) {
	for _, d := range typedDialects {
		e := sqlfn.DivExpr[float64](marksCol, sqlfn.NullIf[float64](maxCol, 0))
		got, err := exprtest.CompileColumns(d, e)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		want := quoteFor(d, `SELECT ("results"."final_marks" / nullif("results"."max_marks", 0.0)) FROM "documents"`)
		if got.SQL != want || len(got.Args) != 0 {
			t.Errorf("%s DivExpr:\n got %s %v\nwant %s", d, got.SQL, got.Args, want)
		}
		got, err = exprtest.CompileColumns(d, sqlfn.Div[float64](marksCol, 2))
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		ph := "?"
		if d == dialect.Postgres {
			ph = "$1"
		}
		want = quoteFor(d, `SELECT ("results"."final_marks" / `+ph+`) FROM "documents"`)
		if got.SQL != want || len(got.Args) != 1 {
			t.Errorf("%s Div:\n got %s %v\nwant %s", d, got.SQL, got.Args, want)
		}
	}
}

// Catches: NullIfExpr dropping an argument.
func TestNullIfExprGolden(t *testing.T) {
	got, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.NullIfExpr[float64](marksCol, maxCol))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT nullif("results"."final_marks", "results"."max_marks") FROM "documents"`
	if got.SQL != want {
		t.Errorf("got %s\nwant %s", got.SQL, want)
	}
}

// Catches: CURRENT_DATE missing on a dialect, and infinity literals rendered
// on a dialect that cannot parse them instead of failing with ErrUnsupported.
func TestCurrentDateAndInfinityGolden(t *testing.T) {
	for _, d := range typedDialects {
		got, err := exprtest.CompileColumns(d, sqlfn.CurrentDate())
		if err != nil || got.SQL != quoteFor(d, `SELECT CURRENT_DATE FROM "documents"`) {
			t.Errorf("%s CURRENT_DATE = %q, %v", d, got.SQL, err)
		}
		for name, e := range map[string]momo.Expr{"infinity": sqlfn.Infinity(), "-infinity": sqlfn.NegInfinity()} {
			got, err := exprtest.CompileColumns(d, e)
			if d == dialect.Postgres {
				want := `SELECT '` + name + `'::timestamptz FROM "documents"`
				if err != nil || got.SQL != want {
					t.Errorf("%s %s = %q, %v; want %s", d, name, got.SQL, err, want)
				}
				continue
			}
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s %s: err = %v, want ErrUnsupported", d, name, err)
			}
		}
	}
}

// Catches: the infinity and date values losing the comparison methods.
func TestInfinityComparison(t *testing.T) {
	at := momo.NewColumn[time.Time]("t", "at", 0)
	got, err := exprtest.Compile(dialect.Postgres, at.LtExpr(sqlfn.Infinity()))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT COUNT(*) FROM "docs" WHERE "t"."at" < 'infinity'::timestamptz`
	if got.SQL != want {
		t.Errorf("got %s\nwant %s", got.SQL, want)
	}
}
