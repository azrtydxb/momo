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

func TestValidExamplesCompile(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/valid/basic")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("valid example failed to build: %v\n%s", err, out)
	}
}
