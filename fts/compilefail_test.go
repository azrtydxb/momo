// SPDX-License-Identifier: Apache-2.0

package fts_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailFullText checks that the full-text builders left the root
// package for fts, and that fts keeps the operand types of its builders. The
// programs live in testdata/compilefail and are built, not run.
func TestCompileFailFullText(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.WebsearchToTSQuery",
			"undefined: momo.PlainToTSQuery",
			"undefined: momo.PhraseToTSQuery",
			"undefined: momo.ToTSQuery",
			"undefined: momo.ToTSVector",
			"undefined: momo.TSMatch",
			"undefined: momo.TSRank",
			"undefined: momo.TSRankCD",
			"undefined: momo.TSHeadline",
			"undefined: momo.TSExpr",
		},
		"wrongtype": {"as Expression[types.TSVector] value in argument to fts.Match"},
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
