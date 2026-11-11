// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
)

type litName string

// Catches: a string with a backslash or NUL reaching the SQL (an injection
// path under MySQL backslash escapes or standard_conforming_strings=off), a
// time of day silently dropped from a date, and either one not failing on
// every dialect.
func TestLitRefusesUnsafeValues(t *testing.T) {
	cases := map[string]momo.Expr{
		"backslash":          momo.Lit(`a\b`),
		"trailing backslash": momo.Lit(`x\`),
		"nul":                momo.Lit("a\x00b"),
		"named backslash":    momo.Lit(litName(`\'`)),
		"time of day":        momo.Lit(time.Date(2026, 3, 4, 23, 59, 0, 0, time.UTC)),
		"one nanosecond":     momo.Lit(time.Date(2026, 3, 4, 0, 0, 0, 1, time.UTC)),
		"midnight not UTC":   momo.Lit(time.Date(2026, 3, 4, 0, 0, 0, 0, time.FixedZone("x", 2*3600))),
	}
	for name, e := range cases {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
			if _, err := exprtest.CompileColumns(d, e); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s on %s: err = %v, want ErrValidation", name, d, err)
			}
		}
	}
}

// Catches: a string Lit in SELECT and GROUP BY rendering differently or
// binding an argument (the issue's COALESCE(col, 'none') case).
func TestLitStringSelectAndGroupByIdentical(t *testing.T) {
	tag := momo.NewColumn[string]("documents", "tag", 0)
	e := func() momo.Expr { return momo.Func[string]("coalesce", tag, momo.Lit("none")) }
	got, err := momo.Select[exprtest.TextTally](exprtest.DB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(e(), momo.CountAll()).GroupBy(e()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT coalesce("documents"."tag", 'none'), COUNT(*) FROM "documents" GROUP BY coalesce("documents"."tag", 'none')`
	if got.SQL != want || len(got.Args) != 0 {
		t.Errorf("got %s %v\nwant %s", got.SQL, got.Args, want)
	}
}
