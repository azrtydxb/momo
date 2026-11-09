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

// TestCompileFailVectorMetricMismatch checks that a bit-vector column cannot
// be given to a dense-vector metric.
func TestCompileFailVectorMetricMismatch(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/compilefail/vector_metric_mismatch")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("vector_metric_mismatch compiled; want a compile error")
	}
	if !strings.Contains(string(out), "does not satisfy") {
		t.Errorf("unexpected error text:\n%s", out)
	}
}

// TestCompileFailTypedExpressions checks that CASE branches share one type
// and that arithmetic accepts numeric columns only.
func TestCompileFailTypedExpressions(t *testing.T) {
	for name, want := range map[string]string{
		"case_mismatch": "cannot use",
		"arith_string":  "does not satisfy",
	} {
		cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/compilefail/"+name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s compiled; want a compile error", name)
			continue
		}
		if !strings.Contains(string(out), want) {
			t.Errorf("%s: want %q in:\n%s", name, want, out)
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

// TestCompileFailArrayPredicatesMoved checks that the array predicates left
// the root package for momo/arrayop, and that the new package keeps the
// element type of the column.
func TestCompileFailArrayPredicatesMoved(t *testing.T) {
	for name, wants := range map[string][]string{
		"arrayop_moved": {
			"undefined: momo.ArrayContains",
			"undefined: momo.ArrayContainedBy",
			"undefined: momo.ArrayOverlaps",
			"undefined: momo.AnyOf",
		},
		"arrayop_wrongtype": {"cannot use"},
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

func TestValidExamplesCompile(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/valid/basic")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("valid example failed to build: %v\n%s", err, out)
	}
}
