// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var pinnedRef = regexp.MustCompile(`@[0-9a-f]{40}$`)

func TestWorkflowsPinned(t *testing.T) {
	dir := filepath.Join(repoRoot(t), ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		w := loadWorkflow(t, e.Name())
		if w.Permissions == nil {
			t.Errorf("%s declares no top-level permissions", e.Name())
		}
		for name, j := range w.Jobs {
			for _, s := range j.Steps {
				if s.Uses == "" || strings.HasPrefix(s.Uses, "./") || strings.HasPrefix(s.Uses, "docker://") {
					continue
				}
				if !pinnedRef.MatchString(s.Uses) {
					t.Errorf("%s job %s: %s is not pinned by a 40-character SHA", e.Name(), name, s.Uses)
				}
			}
		}
	}
}

func TestCodeQLAndGosecRunOnPullRequests(t *testing.T) {
	for _, name := range []string{"codeql.yml", "gosec.yml"} {
		w := loadWorkflow(t, name)
		if _, ok := w.On["pull_request"]; !ok {
			t.Errorf("%s does not run on pull_request", name)
		}
	}
	if _, err := os.Stat(filepath.Join(repoRoot(t), ".github", "dependabot.yml")); err != nil {
		t.Errorf("dependabot.yml missing: %v", err)
	}
	if _, ok := loadWorkflow(t, "scorecard.yml").On["schedule"]; !ok {
		t.Error("scorecard.yml must run on a schedule")
	}
}
