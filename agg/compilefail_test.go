// SPDX-License-Identifier: Apache-2.0

package agg_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailAggregates checks that the ordered aggregates and JSON
// builders left the root package for agg, and that agg keeps the element type
// of the expression. The programs live in testdata/compilefail and are built,
// not run.
func TestCompileFailAggregates(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.AggExpr",
			"undefined: momo.JSONPair",
			"undefined: momo.JSONObjectExpr",
			"undefined: momo.ToJSONExpr",
			"undefined: momo.StringAgg",
			"undefined: momo.ArrayAgg",
			"undefined: momo.JSONBAgg",
			"undefined: momo.PercentileCont",
			"undefined: momo.PercentileDisc",
			"undefined: momo.Aggregate",
			"undefined: momo.JSONField",
			"undefined: momo.JSONBBuildObject",
			"undefined: momo.ToJSONB",
			"undefined: momo.EmptyJSONArray",
		},
		"ordernotexpr": {
			"as momo.Expr value in variable declaration: momo.Order does not implement momo.Expr",
			"as momo.Expr value in argument to agg.JSONB[string]: momo.Order does not implement momo.Expr",
		},
		"wrongtype": {"as momo.Expression[string] value in argument to agg.String"},
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
