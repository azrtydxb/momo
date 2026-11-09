// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// apidiffPin is a go run package reference pinned to a version, never a branch
// or @latest. x/exp is untagged, so the pin is a pseudo-version.
var apidiffPin = regexp.MustCompile(`golang\.org/x/exp/cmd/apidiff@v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.\-]+)?\b`)

// TestAPIDiffDocsGate pins the docs gate: apidiff-docs.yml (pull requests) and ci-agent.yml each have a
// job that runs it, the Makefile runs apidiff at a pinned version through go
// run, the root module does not depend on x/exp, and CONTRIBUTING.md documents
// the escape line.
func TestAPIDiffDocsGate(t *testing.T) {
	for _, name := range []string{"apidiff-docs.yml", "ci-agent.yml"} {
		w := loadWorkflow(t, name)
		j, ok := w.Jobs["apidiff-docs"]
		if !ok {
			t.Errorf("%s has no job apidiff-docs", name)
			continue
		}
		if !runsContain(j, "make apidiff-gate") {
			t.Errorf("%s job apidiff-docs does not run make apidiff-gate", name)
		}
		if j.ContinueOnError == true {
			t.Errorf("%s job apidiff-docs may not fail soft", name)
		}
		fetchesHistory := false
		for _, s := range j.Steps {
			if strings.HasPrefix(s.Uses, "actions/checkout") && s.With["fetch-depth"] == 0 {
				fetchesHistory = true
			}
		}
		if !fetchesHistory {
			t.Errorf("%s job apidiff-docs needs checkout with fetch-depth: 0 to find the merge base", name)
		}
	}

	// The pull request workflow re-runs when the body is edited and reads the
	// live body: the event payload is stale on a re-run.
	pr := loadWorkflow(t, "apidiff-docs.yml")
	types, _ := pr.On["pull_request"].(map[string]any)["types"].([]any)
	hasEdited := false
	for _, ty := range types {
		if ty == "edited" {
			hasEdited = true
		}
	}
	if !hasEdited {
		t.Error("apidiff-docs.yml must trigger on pull_request type edited")
	}
	if !runsContain(pr.Jobs["apidiff-docs"], "gh pr view") {
		t.Error("apidiff-docs.yml must read the live PR body with gh pr view")
	}
	if _, ok := loadWorkflow(t, "ci.yml").Jobs["apidiff-docs"]; ok {
		t.Error("ci.yml must not also define apidiff-docs; it lives in apidiff-docs.yml")
	}

	root := repoRoot(t)
	mk := readText(t, filepath.Join(root, "Makefile"))
	if !apidiffPin.MatchString(mk) {
		t.Error("Makefile does not run golang.org/x/exp/cmd/apidiff at a pinned version")
	}
	if !strings.Contains(mk, "$(GO) run golang.org/x/exp/cmd/apidiff@") {
		t.Error("Makefile must run apidiff through $(GO) run pkg@version")
	}
	if !strings.Contains(mk, "\napidiff-gate:") {
		t.Error("Makefile has no apidiff-gate target")
	}
	if strings.Contains(readText(t, filepath.Join(root, "go.mod")), "golang.org/x/exp") {
		t.Error("go.mod must not require golang.org/x/exp; apidiff runs through go run pkg@version")
	}
	script := readText(t, filepath.Join(root, "scripts", "apidiff-docs-gate.sh"))
	for _, want := range []string{"Docs: no reader-visible change", "README", "llms.txt", "examples/"} {
		if !strings.Contains(script, want) {
			t.Errorf("scripts/apidiff-docs-gate.sh does not mention %q", want)
		}
	}
	if !strings.Contains(readText(t, filepath.Join(root, "CONTRIBUTING.md")), "Docs: no reader-visible change") {
		t.Error("CONTRIBUTING.md does not describe the Docs: no reader-visible change escape line")
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
