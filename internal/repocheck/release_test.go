// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// semverOf parses "vX.Y.Z" into its three numbers.
func semverOf(t *testing.T, v string) [3]int {
	t.Helper()
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		t.Fatalf("%q is not vX.Y.Z", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("%q is not vX.Y.Z: %v", v, err)
		}
		out[i] = n
	}
	return out
}

// TestOtelmomoIsReleasable: otelmomo is the first nested module that users
// import, so it needs its own release tag (otelmomo/vX.Y.Z) and a require on a
// real, already released momo version. A consumer ignores the replace
// directive, so a zero pseudo-version would make the module unusable.
func TestOtelmomoIsReleasable(t *testing.T) {
	root := repoRoot(t)
	var cfg struct {
		Packages map[string]struct {
			Component             string `json:"component"`
			IncludeComponentInTag bool   `json:"include-component-in-tag"`
			TagSeparator          string `json:"tag-separator"`
		} `json:"packages"`
	}
	b, err := os.ReadFile(filepath.Join(root, "release-please-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	pkg, ok := cfg.Packages["otelmomo"]
	if !ok || pkg.Component != "otelmomo" || !pkg.IncludeComponentInTag || pkg.TagSeparator != "/" {
		t.Errorf("release-please-config.json needs an otelmomo package with component otelmomo, include-component-in-tag and tag-separator \"/\": %+v", pkg)
	}
	var manifest map[string]string
	mb, err := os.ReadFile(filepath.Join(root, ".release-please-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &manifest); err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest["otelmomo"]; !ok {
		t.Error(".release-please-manifest.json has no otelmomo entry")
	}

	gm, err := os.ReadFile(filepath.Join(root, "otelmomo", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\s*github\.com/azrtydxb/momo (v\S+)\s*$`).FindSubmatch(gm)
	if m == nil {
		t.Fatal("otelmomo/go.mod has no require of github.com/azrtydxb/momo")
	}
	req := string(m[1])
	if strings.HasPrefix(req, "v0.0.0-") {
		t.Fatalf("otelmomo/go.mod requires momo %s, a pseudo-version no consumer can resolve; require a released version", req)
	}
	released, got := semverOf(t, "v"+manifest["."]), semverOf(t, req)
	for i := range got {
		if got[i] != released[i] {
			if got[i] > released[i] {
				t.Fatalf("otelmomo/go.mod requires momo %s, newer than the released %s in the manifest", req, manifest["."])
			}
			break
		}
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
