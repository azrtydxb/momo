// SPDX-License-Identifier: Apache-2.0

package jsonop_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailJSONOperators checks that the JSON operators left the root
// package for jsonop, and that jsonop keeps the type of the key. The programs
// live in testdata/compilefail and are built, not run.
func TestCompileFailJSONOperators(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.JSONText",
			"undefined: momo.JSONPath",
			"undefined: momo.JSONContains",
			"undefined: momo.JSONHasKey",
			"undefined: momo.JSONTextExpr",
		},
		"wrongtype": {"cannot use 42 (untyped int constant) as string value in argument to jsonop.Text"},
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
