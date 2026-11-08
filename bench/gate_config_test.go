// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestBenchGateConfig reads the CI workflow of the allocation gate. Actions are
// pinned by version tag (@vN), the azrtydxb standard recorded in
// .procoder/ask/decisions.md, not by commit SHA as the plan text said.
func TestBenchGateConfig(t *testing.T) {
	raw, err := os.ReadFile("../.github/workflows/bench-gate.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	job, ok := wf.Jobs["allocs"]
	if !ok {
		t.Fatal("job allocs missing")
	}
	var script strings.Builder
	tag := regexp.MustCompile(`@v[0-9]+$`)
	for _, s := range job.Steps {
		script.WriteString(s.Run + "\n")
		if s.Uses != "" && !tag.MatchString(s.Uses) {
			t.Errorf("action %s is not pinned by version tag (@vN)", s.Uses)
		}
	}
	for _, want := range []string{
		"benchstat", "-filter '.unit:allocs/op'", "benchgate", "-max-percent 5", "bench/baseline/allocs.txt",
	} {
		if !strings.Contains(script.String(), want) {
			t.Errorf("gate script lacks %q", want)
		}
	}
	// Timings are printed for information only; the gate input is the allocs/op filter.
	for _, line := range strings.Split(script.String(), "\n") {
		if strings.Contains(line, "benchgate") && !strings.Contains(line, "delta.txt") {
			t.Errorf("benchgate must read the allocs/op delta file, got %q", line)
		}
		if strings.Contains(line, "benchgate") && strings.Contains(line, "ns/op") {
			t.Error("timings must not be gated in CI")
		}
	}
}
