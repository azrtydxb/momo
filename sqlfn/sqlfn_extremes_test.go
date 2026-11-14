// SPDX-License-Identifier: Apache-2.0

package sqlfn_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
)

var (
	xtA     = momo.NewColumn[int64]("documents", "a", 0)
	xtB     = momo.NewColumn[int64]("documents", "b", 0)
	xtC     = momo.NewColumn[int64]("documents", "c", 0)
	xtScore = momo.NewColumn[float64]("documents", "score", 0)
)

// Catches: GREATEST and LEAST rendered with the MySQL or SQLite NULL rule (a
// NULL argument making the result NULL) instead of PostgreSQL's NULL-ignoring
// one, the SQLite call using the one-argument aggregate max(), or an argument
// missing from the rewrite.
func TestGreatestLeastGolden(t *testing.T) {
	two := func(least bool) momo.Expr {
		if least {
			return sqlfn.Least[int64](xtA, xtB)
		}
		return sqlfn.Greatest[int64](xtA, xtB)
	}
	three := sqlfn.Greatest[int64](xtA, xtB, xtC)
	for _, c := range []struct {
		name string
		d    dialect.Name
		e    momo.Expr
		want string
	}{
		{"pg greatest", dialect.Postgres, two(false), `SELECT GREATEST("documents"."a", "documents"."b") FROM "documents"`},
		{"pg least", dialect.Postgres, two(true), `SELECT LEAST("documents"."a", "documents"."b") FROM "documents"`},
		{"pg three", dialect.Postgres, three, `SELECT GREATEST("documents"."a", "documents"."b", "documents"."c") FROM "documents"`},
		{"mysql greatest", dialect.MySQL, two(false),
			`SELECT GREATEST(COALESCE("documents"."a", "documents"."b"), COALESCE("documents"."b", "documents"."a")) FROM "documents"`},
		{"mysql least", dialect.MySQL, two(true),
			`SELECT LEAST(COALESCE("documents"."a", "documents"."b"), COALESCE("documents"."b", "documents"."a")) FROM "documents"`},
		{"mysql three", dialect.MySQL, three,
			`SELECT GREATEST(COALESCE("documents"."a", "documents"."b", "documents"."c"), COALESCE("documents"."b", "documents"."a", "documents"."c"), COALESCE("documents"."c", "documents"."a", "documents"."b")) FROM "documents"`},
		{"sqlite greatest", dialect.SQLite, two(false),
			`SELECT max(COALESCE("documents"."a", "documents"."b"), COALESCE("documents"."b", "documents"."a")) FROM "documents"`},
		{"sqlite least", dialect.SQLite, two(true),
			`SELECT min(COALESCE("documents"."a", "documents"."b"), COALESCE("documents"."b", "documents"."a")) FROM "documents"`},
	} {
		got, err := exprtest.CompileColumns(c.d, c.e)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if want := quoteFor(c.d, c.want); got.SQL != want || len(got.Args) != 0 {
			t.Errorf("%s:\n got %s %v\nwant %s", c.name, got.SQL, got.Args, want)
		}
	}
}

// Catches: a bound value argument rendered once instead of once per rewrite
// operand on SQLite and MySQL, so the placeholders and the arguments disagree.
func TestGreatestBoundValueArgsGolden(t *testing.T) {
	e := sqlfn.Greatest[int64](momo.Value(int64(1)), xtA)
	got, err := exprtest.CompileColumns(dialect.MySQL, e)
	want := "SELECT GREATEST(COALESCE(?, `documents`.`a`), COALESCE(`documents`.`a`, ?)) FROM `documents`"
	if err != nil || got.SQL != want || len(got.Args) != 2 {
		t.Errorf("got %s %v (%v)\nwant %s with 2 args", got.SQL, got.Args, err, want)
	}
	got, err = exprtest.CompileColumns(dialect.Postgres, e)
	want = `SELECT GREATEST($1, "documents"."a") FROM "documents"`
	if err != nil || got.SQL != want || len(got.Args) != 1 {
		t.Errorf("got %s %v (%v)\nwant %s with 1 arg", got.SQL, got.Args, err, want)
	}
}

// Catches: GREATEST or LEAST over types.Numeric silently ordering TEXT
// lexically on SQLite and MySQL, or refused on PostgreSQL.
func TestGreatestLeastNumeric(t *testing.T) {
	g := sqlfn.Greatest[types.Numeric](costCol, limitCol)
	l := sqlfn.Least[types.Numeric](costCol, limitCol)
	got, err := exprtest.CompileColumns(dialect.Postgres, g)
	if want := `SELECT GREATEST("documents"."cost", "documents"."cost_limit") FROM "documents"`; err != nil || got.SQL != want {
		t.Errorf("pg greatest = %s (%v), want %s", got.SQL, err, want)
	}
	if _, err := exprtest.CompileColumns(dialect.Postgres, l); err != nil {
		t.Errorf("pg least: %v", err)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for name, e := range map[string]momo.Expr{"greatest": g, "least": l} {
			_, err := exprtest.CompileColumns(d, e)
			if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "PostgreSQL only") {
				t.Errorf("%s %s = %v, want ErrUnsupported naming PostgreSQL only", d, name, err)
			}
		}
	}
}

// Catches: Floor or Ceil rendered with a function SQLite may lack (the mattn
// driver has no math functions without a build tag), the SQLite rewrite
// dropping a branch, or the other dialects not calling floor and ceil.
func TestFloorCeilGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		d    dialect.Name
		e    momo.Expr
		want string
	}{
		{"pg floor", dialect.Postgres, sqlfn.Floor[float64](xtScore), `SELECT floor("documents"."score") FROM "documents"`},
		{"pg ceil", dialect.Postgres, sqlfn.Ceil[float64](xtScore), `SELECT ceil("documents"."score") FROM "documents"`},
		{"mysql floor", dialect.MySQL, sqlfn.Floor[float64](xtScore), `SELECT floor("documents"."score") FROM "documents"`},
		{"mysql ceil", dialect.MySQL, sqlfn.Ceil[float64](xtScore), `SELECT ceil("documents"."score") FROM "documents"`},
		{"sqlite floor", dialect.SQLite, sqlfn.Floor[float64](xtScore),
			`SELECT CASE WHEN abs(("documents"."score")) >= 4503599627370496.0 THEN ("documents"."score") ELSE CAST(CAST(("documents"."score") AS INTEGER) - (("documents"."score") < CAST(("documents"."score") AS INTEGER)) AS REAL) END FROM "documents"`},
		{"sqlite ceil", dialect.SQLite, sqlfn.Ceil[float64](xtScore),
			`SELECT CASE WHEN abs(("documents"."score")) >= 4503599627370496.0 THEN ("documents"."score") ELSE CAST(CAST(("documents"."score") AS INTEGER) + (("documents"."score") > CAST(("documents"."score") AS INTEGER)) AS REAL) END FROM "documents"`},
	} {
		got, err := exprtest.CompileColumns(c.d, c.e)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if want := quoteFor(c.d, c.want); got.SQL != want || len(got.Args) != 0 {
			t.Errorf("%s:\n got %s %v\nwant %s", c.name, got.SQL, got.Args, want)
		}
	}
}

// Catches: Floor or Ceil over types.Numeric rendered on SQLite and MySQL,
// where the TEXT column would go through floating point, or refused on
// PostgreSQL where numeric floors exactly.
func TestFloorCeilNumeric(t *testing.T) {
	got, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.Floor[types.Numeric](costCol))
	if want := `SELECT floor("documents"."cost") FROM "documents"`; err != nil || got.SQL != want {
		t.Errorf("pg floor = %s (%v), want %s", got.SQL, err, want)
	}
	got, err = exprtest.CompileColumns(dialect.Postgres, sqlfn.Ceil[types.Numeric](costCol))
	if want := `SELECT ceil("documents"."cost") FROM "documents"`; err != nil || got.SQL != want {
		t.Errorf("pg ceil = %s (%v), want %s", got.SQL, err, want)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for name, e := range map[string]momo.Expr{
			"floor": sqlfn.Floor[types.Numeric](costCol),
			"ceil":  sqlfn.Ceil[types.Numeric](costCol),
		} {
			_, err := exprtest.CompileColumns(d, e)
			if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "PostgreSQL only") {
				t.Errorf("%s %s = %v, want ErrUnsupported naming PostgreSQL only", d, name, err)
			}
		}
	}
}
