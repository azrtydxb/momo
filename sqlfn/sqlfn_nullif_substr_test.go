// SPDX-License-Identifier: Apache-2.0

package sqlfn_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/sqlfn"
)

var (
	substrFromCol = momo.NewColumn[int32]("documents", "start_pos", 0)
	substrLenCol  = momo.NewColumn[int32]("documents", "len", 0)
)

// Catches: NullIf refusing a string operand (the constraint was numeric
// only), or binding the string instead of writing it as a literal.
func TestNullIfStringGolden(t *testing.T) {
	got, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.NullIf(docTitle, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT nullif("documents"."title", '') FROM "documents"`
	if got.SQL != want || len(got.Args) != 0 {
		t.Errorf("NullIf string = %s %v, want %s", got.SQL, got.Args, want)
	}
}

// Catches: NullIf rendering a bool operand as a bound parameter or wrongly.
func TestNullIfBoolGolden(t *testing.T) {
	got, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.NullIf(docPublished, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Args) != 0 {
		t.Errorf("NullIf bool bound its operand: %s %v", got.SQL, got.Args)
	}
}

// Catches: SubstrExpr rendering its positions as literals or parameters
// instead of the expressions it was given, and SubstrToEnd losing its
// two-argument form.
func TestSubstrExprAndToEndGolden(t *testing.T) {
	for _, d := range typedDialects {
		got, err := exprtest.CompileColumns(d, sqlfn.SubstrExpr(docTitle, substrFromCol, substrLenCol))
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		want := quoteFor(d, `SELECT substr("documents"."title", "documents"."start_pos", "documents"."len") FROM "documents"`)
		if got.SQL != want || len(got.Args) != 0 {
			t.Errorf("%s SubstrExpr = %s %v, want %s", d, got.SQL, got.Args, want)
		}
		got, err = exprtest.CompileColumns(d, sqlfn.SubstrToEnd(docTitle, 4))
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		want = quoteFor(d, `SELECT substr("documents"."title", 4) FROM "documents"`)
		if got.SQL != want || len(got.Args) != 0 {
			t.Errorf("%s SubstrToEnd = %s %v, want %s", d, got.SQL, got.Args, want)
		}
	}
}
