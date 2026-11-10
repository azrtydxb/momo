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
	files := []string{"env.json", "versions.txt", "raw.txt", "summary.txt"}
	if os.Getenv("BENCH_PG_URL") != "" {
		// The pgx vector codec benchmark (spec S-50) is recorded with the run.
		files = append(files, "vector-encoding.txt")
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(matches[0], f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
}
