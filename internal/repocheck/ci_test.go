// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type workflow struct {
	On          map[string]any `yaml:"on"`
	Permissions any            `yaml:"permissions"`
	Jobs        map[string]job `yaml:"jobs"`
}

type job struct {
	RunsOn          any    `yaml:"runs-on"`
	If              string `yaml:"if"`
	ContinueOnError any    `yaml:"continue-on-error"`
	Permissions     any    `yaml:"permissions"`
	Strategy        struct {
		Matrix map[string]any `yaml:"matrix"`
	} `yaml:"strategy"`
	Services map[string]struct {
		Image string `yaml:"image"`
	} `yaml:"services"`
	Steps []step `yaml:"steps"`
}

type step struct {
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}

func loadWorkflow(t *testing.T, name string) workflow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return w
}

func matrixHas(v any, want string) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, x := range list {
		if s, ok := x.(string); ok && s == want {
			return true
		}
	}
	return false
}

func runsContain(j job, sub string) bool {
	for _, s := range j.Steps {
		if strings.Contains(s.Run, sub) {
			return true
		}
	}
	return false
}

func usesAction(j job, prefix string) bool {
	for _, s := range j.Steps {
		if strings.HasPrefix(s.Uses, prefix) {
			return true
		}
	}
	return false
}

func TestCIWorkflowCoverage(t *testing.T) {
	w := loadWorkflow(t, "ci.yml")
	required := map[string]string{
		"unit":         "make test",
		"compile-fail": "make test-compilefail",
		"race":         "make test-race",
		"fuzz-smoke":   "make test-fuzz",
		"integration":  "make test-integration-",
	}
	for name, cmd := range required {
		j, ok := w.Jobs[name]
		if !ok {
			t.Errorf("ci.yml has no job %q", name)
			continue
		}
		if !runsContain(j, cmd) {
			t.Errorf("job %q does not run %q", name, cmd)
		}
		if j.If != "" || j.ContinueOnError == true {
			t.Errorf("job %q may be skipped", name)
		}
		if !matrixHas(j.Strategy.Matrix["go"], "1.26") || !matrixHas(j.Strategy.Matrix["go"], "1.27") {
			t.Errorf("job %q does not test Go 1.26 and 1.27", name)
		}
	}
	integ := w.Jobs["integration"]
	// Every integration job runs `go test -tags integration -race ./...` with
	// MOMO_REQUIRE_INTEGRATION=1, directly or through its make target.
	for name, j := range w.Jobs {
		if !strings.HasPrefix(name, "integration") {
			continue
		}
		for _, s := range j.Steps {
			if !strings.Contains(s.Run, "make test-integration-") && !strings.Contains(s.Run, "go test") {
				continue
			}
			cmd := s.Run
			if strings.Contains(cmd, "make test-integration-") {
				var dialects []string
				list, ok := j.Strategy.Matrix["dialect"].([]any)
				if !ok {
					t.Fatalf("job %q runs make test-integration-* but its matrix has no dialect list", name)
				}
				for _, d := range list {
					ds, ok := d.(string)
					if !ok {
						t.Fatalf("job %q: dialect matrix entry %v is %T, want string", name, d, d)
					}
					dialects = append(dialects, ds)
				}
				for _, d := range dialects {
					c := exec.Command("make", "-n", "test-integration-"+d)
					c.Dir = repoRoot(t)
					out, err := c.CombinedOutput()
					if err != nil {
						t.Fatalf("make -n test-integration-%s: %v\n%s", d, err, out)
					}
					cmd = string(out)
					for _, want := range []string{"-tags integration", "-race", "MOMO_REQUIRE_INTEGRATION=1"} {
						if !strings.Contains(cmd, want) {
							t.Errorf("job %q dialect %s: command lacks %q:\n%s", name, d, want, cmd)
						}
					}
				}
				continue
			}
			for _, want := range []string{"-tags integration", "-race", "MOMO_REQUIRE_INTEGRATION=1"} {
				if !strings.Contains(cmd, want) {
					t.Errorf("job %q step lacks %q: %s", name, want, cmd)
				}
			}
		}
	}
	if !matrixHas(integ.Strategy.Matrix["dialect"], "postgres") {
		t.Error("the integration matrix does not enable postgres")
	}
	if pg := integ.Services["postgres"].Image; !strings.HasPrefix(pg, "pgvector/pgvector:") {
		t.Errorf("postgres service image = %q, want pgvector/pgvector", pg)
	}
	if !matrixHas(integ.Strategy.Matrix["pg"], "0.8.7-pg17") || !matrixHas(integ.Strategy.Matrix["pg"], "0.8.7-pg16") {
		t.Error("the integration matrix must test pgvector pg17 and one older major")
	}
	for _, name := range []string{"windows", "macos"} {
		if _, ok := w.Jobs[name]; !ok {
			t.Errorf("ci.yml has no %s job", name)
		}
	}
	if w.Jobs["windows"].RunsOn != "windows-latest" || w.Jobs["macos"].RunsOn != "macos-latest" {
		t.Error("windows and macos jobs must run on windows-latest and macos-latest")
	}
	if !usesAction(w.Jobs["govulncheck"], "golang/govulncheck-action@") {
		t.Error("govulncheck job must use the pinned govulncheck action")
	}
}
