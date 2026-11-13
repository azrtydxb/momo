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
	costCol  = momo.NewColumn[types.Numeric]("documents", "cost", 0)
	limitCol = momo.NewColumn[types.Numeric]("documents", "cost_limit", 0)
)

// Catches: a types.Numeric operand that does not compile or renders SQL other
// than the parenthesised operator, on PostgreSQL.
func TestNumericArithmeticGoldenPostgres(t *testing.T) {
	cases := []struct {
		name string
		e    momo.Expr
		want string
		args int
	}{
		{"add", sqlfn.Add(costCol, types.Numeric{Decimal: "0.000001"}), `("documents"."cost" + $1)`, 1},
		{"sub", sqlfn.Sub(costCol, types.Numeric{Decimal: "1"}), `("documents"."cost" - $1)`, 1},
		{"mul", sqlfn.Mul(costCol, types.Numeric{Decimal: "2"}), `("documents"."cost" * $1)`, 1},
		{"div", sqlfn.Div(costCol, types.Numeric{Decimal: "4"}), `("documents"."cost" / $1)`, 1},
		{"addexpr", sqlfn.AddExpr[types.Numeric](costCol, limitCol), `("documents"."cost" + "documents"."cost_limit")`, 0},
	}
	for _, c := range cases {
		got, err := exprtest.CompileColumns(dialect.Postgres, c.e)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want := "SELECT " + c.want + ` FROM "documents"`
		if got.SQL != want || len(got.Args) != c.args {
			t.Errorf("%s:\n got %s %v\nwant %s", c.name, got.SQL, got.Args, want)
		}
	}
}

// Catches: types.Numeric arithmetic silently rendered on SQLite and MySQL,
// where the column is TEXT and + would go through floating point.
func TestNumericArithmeticRefusedOffPostgres(t *testing.T) {
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for op, e := range map[string]momo.Expr{
			"+": sqlfn.Add(costCol, types.Numeric{Decimal: "1"}),
			"-": sqlfn.SubExpr[types.Numeric](costCol, limitCol),
			"*": sqlfn.Mul(costCol, types.Numeric{Decimal: "1"}),
			"/": sqlfn.DivExpr[types.Numeric](costCol, limitCol),
		} {
			_, err := exprtest.CompileColumns(d, e)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s %s = %v, want ErrUnsupported", d, op, err)
				continue
			}
			if !strings.Contains(err.Error(), "PostgreSQL only") || !strings.Contains(err.Error(), `"`+op+`"`) {
				t.Errorf("%s %s: error %q does not name the operator and PostgreSQL only", d, op, err)
			}
		}
	}
}

// Catches: the new constraint breaking the plain numeric kinds on SQLite.
func TestPlainNumericStillWorksOnSQLite(t *testing.T) {
	if _, err := exprtest.CompileColumns(dialect.SQLite, sqlfn.Add[float64](marksCol, 1)); err != nil {
		t.Fatalf("float64 Add on SQLite: %v", err)
	}
}

// Catches: NullIf on a types.Numeric rendering a bound or broken literal on
// PostgreSQL, accepting one on SQLite or MySQL, or accepting a bad decimal.
func TestNullIfNumeric(t *testing.T) {
	got, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.NullIf(costCol, types.Numeric{Decimal: "0"}))
	want := `SELECT nullif("documents"."cost", '0'::numeric) FROM "documents"`
	if err != nil || got.SQL != want || len(got.Args) != 0 {
		t.Errorf("NullIf = %s %v (%v), want %s", got.SQL, got.Args, err, want)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if _, err := exprtest.CompileColumns(d, sqlfn.NullIf(costCol, types.Numeric{})); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s NullIf = %v, want ErrUnsupported", d, err)
		}
	}
	_, err = exprtest.CompileColumns(dialect.Postgres, sqlfn.NullIf(costCol, types.Numeric{Decimal: "1'; DROP"}))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("NullIf with a bad decimal = %v, want ErrValidation", err)
	}
}

var (
	smallCol = momo.NewColumn[uint8]("documents", "small", 0)
	ratioCol = momo.NewColumn[float32]("documents", "ratio", 0)
)

// Catches: NullIf's own literal rendering drifting from what momo.Lit wrote
// for the plain kinds before types.Numeric joined the constraint (a negative
// not parenthesised, a float32 widened to float64 digits, a missing ".0").
func TestNullIfPlainKindsMatchLit(t *testing.T) {
	for _, c := range []struct {
		name string
		e    momo.Expr
		want string
	}{
		{"int64", sqlfn.NullIf(docTenantID, int64(-1)), `SELECT nullif("documents"."tenant_id", (-1)) FROM "documents"`},
		{"uint8", sqlfn.NullIf(smallCol, uint8(7)), `SELECT nullif("documents"."small", 7) FROM "documents"`},
		{"float32", sqlfn.NullIf(ratioCol, float32(1.5)), `SELECT nullif("documents"."ratio", 1.5) FROM "documents"`},
	} {
		got, err := exprtest.CompileColumns(dialect.Postgres, c.e)
		if err != nil || got.SQL != c.want || len(got.Args) != 0 {
			t.Errorf("%s: got %s %v (%v)\nwant %s", c.name, got.SQL, got.Args, err, c.want)
		}
	}
}

// Catches: a NaN Numeric literal in NullIf rendered wrongly or refused.
func TestNullIfNumericNaN(t *testing.T) {
	got, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.NullIf(costCol, types.Numeric{NaN: true}))
	want := `SELECT nullif("documents"."cost", 'NaN'::numeric) FROM "documents"`
	if err != nil || got.SQL != want {
		t.Errorf("got %s (%v), want %s", got.SQL, err, want)
	}
}

// Catches: a Numeric with conflicting flags reaching the SQL text instead of
// failing with ErrValidation.
func TestNullIfNumericConflictingFlags(t *testing.T) {
	_, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.NullIf(costCol, types.Numeric{NaN: true, Infinity: true}))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("conflicting flags = %v, want ErrValidation", err)
	}
}

// Catches: a nested types.Numeric arithmetic expression escaping the dialect
// check on SQLite.
func TestNestedNumericArithmeticRefusedOnSQLite(t *testing.T) {
	n := types.Numeric{Decimal: "1"}
	_, err := exprtest.CompileColumns(dialect.SQLite, sqlfn.Add(sqlfn.Add(costCol, n), n))
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("nested Add on SQLite = %v, want ErrUnsupported", err)
	}
}
