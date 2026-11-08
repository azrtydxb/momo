// SPDX-License-Identifier: Apache-2.0

package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBenchResultsLayout(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	out := t.TempDir()
	cmd := exec.Command("bash", "run.sh", "smoke")
	cmd.Env = append(os.Environ(), "BENCH_RESULTS_DIR="+out)
	cmd.Dir = "."
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run.sh smoke: %v\n%s", err, b)
	}
	matches, err := filepath.Glob(filepath.Join(out, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("want one run directory, got %v", matches)
	}
	for _, f := range []string{"env.json", "versions.txt", "raw.txt", "summary.txt"} {
		if _, err := os.Stat(filepath.Join(matches[0], f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
}
