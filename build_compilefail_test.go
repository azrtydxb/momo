// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCompileFail(t *testing.T) {
	for _, name := range []string{"wrongtype", "wrongtype_any"} {
		cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/compilefail/"+name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s compiled; want a compile error", name)
			continue
		}
		if !strings.Contains(string(out), "cannot use") {
			t.Errorf("%s: unexpected error text:\n%s", name, out)
		}
	}
}

// TestCompileFailPermissionConversions checks that the permission wrappers
// cannot be converted into one another or into Column, which would bypass
// their missing or restricted Set.
func TestCompileFailPermissionConversions(t *testing.T) {
	for _, name := range []string{
		"convert_readonly_to_column",
		"convert_immutable_to_column",
		"convert_readonly_to_immutable",
		"convert_nullreadonly_to_readonly",
	} {
		cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/compilefail/"+name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s compiled; want a compile error", name)
			continue
		}
		if !strings.Contains(string(out), "cannot convert") {
			t.Errorf("%s: want %q in:\n%s", name, "cannot convert", out)
		}
	}
}

func TestValidExamplesCompile(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/valid/basic")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("valid example failed to build: %v\n%s", err, out)
	}
}
