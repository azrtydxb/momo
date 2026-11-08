// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestBenchGateConfig reads the places that wire the allocation gate: the
// bench-module job of both CI workflows must run `make bench-gate`, and the
// Makefile recipe must run under bash with pipefail (a benchmark run that fails
// behind a `| tee` would otherwise pass) and feed benchgate the allocs/op
// comparison only.
func TestBenchGateConfig(t *testing.T) {
	for _, path := range []string{"../.github/workflows/ci.yml", "../.github/workflows/ci-agent.yml"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var wf struct {
			Jobs map[string]struct {
				Steps []struct {
					Run string `yaml:"run"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			t.Fatal(err)
		}
		job, ok := wf.Jobs["bench-module"]
		if !ok {
			t.Fatalf("%s: job bench-module missing", path)
		}
		found := false
		for _, s := range job.Steps {
			if strings.TrimSpace(s.Run) == "make bench-gate" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: bench-module does not run make bench-gate", path)
		}
	}

	mk, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(mk)
	for _, want := range []string{"SHELL := bash", "-eo pipefail"} {
		if !strings.Contains(makefile, want) {
			t.Errorf("Makefile lacks %q: a failed benchmark run behind tee would pass", want)
		}
	}
	start := strings.Index(makefile, "\nbench-gate:\n")
	if start < 0 {
		t.Fatal("Makefile has no bench-gate target")
	}
	recipe := makefile[start+1:]
	if end := strings.Index(recipe, "\n\n"); end >= 0 {
		recipe = recipe[:end]
	}
	for _, want := range []string{
		"-bench='BenchmarkAllocs_'", "$(BENCHSTAT)", "-filter '.unit:allocs/op'", "benchgate", "-max-percent 5",
		"-baseline baseline/allocs.txt", "-new",
	} {
		if !strings.Contains(recipe, want) {
			t.Errorf("bench-gate recipe lacks %q", want)
		}
	}
	// Timings are never gated; the gate input is the allocs/op filter.
	for _, line := range strings.Split(recipe, "\n") {
		if strings.Contains(line, "benchgate") && !strings.Contains(line, "delta.txt") {
			t.Errorf("benchgate must read the allocs/op delta file, got %q", line)
		}
		if strings.Contains(line, "benchgate") && strings.Contains(line, "ns/op") {
			t.Error("timings must not be gated in CI")
		}
	}
}
