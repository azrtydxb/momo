// SPDX-License-Identifier: Apache-2.0

package arrayop_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailArrayPredicates checks that the array predicates left the
// root package for arrayop, and that arrayop keeps the element type of the
// column. The programs live in testdata/compilefail and are built, not run.
func TestCompileFailArrayPredicates(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.ArrayContains",
			"undefined: momo.ArrayContainedBy",
			"undefined: momo.ArrayOverlaps",
			"undefined: momo.AnyOf",
		},
		"wrongtype": {"cannot use 42 (untyped int constant) as string value in argument to arrayop.Contains"},
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
