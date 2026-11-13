// SPDX-License-Identifier: Apache-2.0

package sqlfn_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailSQLFunctions checks that the scalar SQL functions left the
// root package for sqlfn, that CASE branches share one type, and that
// arithmetic accepts numeric columns only (a string or a types.Date column is
// refused; types.Numeric is accepted). The programs live in
// testdata/compilefail and are built, not run.
func TestCompileFailSQLFunctions(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.Substr",
			"undefined: momo.SubstrFrom",
			"undefined: momo.Lower",
			"undefined: momo.Upper",
			"undefined: momo.Length",
			"undefined: momo.CurrentSchema",
			"undefined: momo.Case",
			"undefined: momo.Add",
			"undefined: momo.AddExpr",
			"undefined: momo.Sub",
			"undefined: momo.SubExpr",
			"undefined: momo.Mul",
			"undefined: momo.MulExpr",
			"undefined: momo.CoalesceExpr",
			"undefined: momo.Cast",
			"undefined: momo.Concat",
			"undefined: momo.CaseExpr",
			"undefined: momo.ArithExpr",
			"undefined: momo.CastExpr",
			"undefined: momo.LengthExpr",
			"undefined: momo.ConcatExpr",
			"undefined: momo.CurrentSchemaExpr",
			"undefined: momo.Numeric",
		},
		"casemismatch": {"cannot use 42 (untyped int constant) as string value"},
		"arithstring":  {"does not satisfy sqlfn.Numeric"},
		"arithdate":    {"does not satisfy sqlfn.Numeric"},
	} {
		cmd := exec.Command("go", "build", "-gcflags=-e", "-o", os.DevNull, "./testdata/compilefail/"+name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s compiled; want a compile error", name)
			continue
		}
		for _, want := range wants {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s: want %q in:\n%s", name, want, out)
			}
		}
	}
}
