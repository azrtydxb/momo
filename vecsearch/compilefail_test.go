// SPDX-License-Identifier: Apache-2.0

package vecsearch_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailSearchSettings checks that the search settings left the root
// package for vecsearch. The program lives in testdata/compilefail and is
// built, not run.
func TestCompileFailSearchSettings(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.SearchSettings",
			"undefined: momo.IterativeRelaxedOrder",
			"db.WithSearch undefined",
			"tx.SetSearch undefined",
		},
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
