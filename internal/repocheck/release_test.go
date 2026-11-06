// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseConfig(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "release-please-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["release-type"] != "go" {
		t.Errorf("release-type = %v, want go", cfg["release-type"])
	}
	if cfg["bump-minor-pre-major"] != true {
		t.Error("bump-minor-pre-major must be true: feat! bumps the minor before 1.0")
	}
	if cfg["changelog-path"] != "CHANGELOG.md" {
		t.Errorf("changelog-path = %v", cfg["changelog-path"])
	}
	if cfg["include-v-in-tag"] != true {
		t.Error("tags must be vX.Y.Z")
	}
	m, err := os.ReadFile(filepath.Join(root, ".release-please-manifest.json"))
	if err != nil || !strings.Contains(string(m), `"."`) {
		t.Errorf("manifest missing root package: %v", err)
	}
	w := loadWorkflow(t, "pr-title.yml")
	if _, ok := w.On["pull_request"]; !ok {
		t.Error("pr-title.yml must run on pull_request")
	}
	if !strings.Contains(stepUses(w), "amannn/action-semantic-pull-request@v") {
		t.Error("pr-title.yml must use the semantic PR action by version tag")
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "release-please.yml")); err != nil {
		t.Error(err)
	}
}

func stepUses(w workflow) string {
	var out strings.Builder
	for _, j := range w.Jobs {
		for _, s := range j.Steps {
			out.WriteString(s.Uses + "\n")
		}
	}
	return out.String()
}
