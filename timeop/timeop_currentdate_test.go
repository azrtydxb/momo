// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/timeop"
)

// Catches: CURRENT_DATE missing on a dialect, and infinity literals rendered
// on a dialect that cannot parse them instead of failing with ErrUnsupported.
func TestCurrentDateAndInfinityGolden(t *testing.T) {
	for _, d := range typedDialects {
		got, err := selectOne(d, timeop.CurrentDate())
		if err != nil || got.SQL != quoteFor(d, `SELECT CURRENT_DATE FROM "documents"`) {
			t.Errorf("%s CURRENT_DATE = %q, %v", d, got.SQL, err)
		}
		for name, e := range map[string]momo.Expr{"infinity": timeop.Infinity(), "-infinity": timeop.NegInfinity()} {
			got, err := selectOne(d, e)
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

// Catches: the infinity value losing the comparison methods of a timestamp
// expression.
func TestInfinityComparison(t *testing.T) {
	got, err := exprtest.Compile(dialect.Postgres, tmCreated.LtExpr(timeop.Infinity()))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT COUNT(*) FROM "documents" WHERE "documents"."created_at" < 'infinity'::timestamptz`
	if got.SQL != want {
		t.Errorf("got %s\nwant %s", got.SQL, want)
	}
}
