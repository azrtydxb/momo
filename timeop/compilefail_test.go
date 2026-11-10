// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailTimeExpressions checks that the date and time expressions
// left the root package for timeop, and that timeop keeps the timestamp type
// of its operand. The programs live in testdata/compilefail and are built, not
// run.
func TestCompileFailTimeExpressions(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.Extract",
			"undefined: momo.ExtractYear",
			"undefined: momo.AtTimeZone",
			"undefined: momo.DateTrunc",
			"undefined: momo.TruncDay",
			"undefined: momo.AddInterval",
			"undefined: momo.SubInterval",
			"undefined: momo.IntervalOf",
			"undefined: momo.IntervalDay",
			"undefined: momo.Now",
			"undefined: momo.CurrentTimestamp",
			"undefined: momo.TimeExpr",
		},
		"wrongtype": {"value in argument to timeop.Extract"},
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
