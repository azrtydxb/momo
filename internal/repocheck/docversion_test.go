// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const versionMarker = "x-release-please-version"

// TestDocsNameManifestVersion keeps the version strings in the docs equal to
// the released version. Release-please rewrites every line that carries the
// marker, but only in the files listed under extra-files, so the test also
// pins that list.
func TestDocsNameManifestVersion(t *testing.T) {
	root := repoRoot(t)

	mb, err := os.ReadFile(filepath.Join(root, ".release-please-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(mb, &manifest); err != nil {
		t.Fatal(err)
	}
	version := manifest["."]
	if version == "" {
		t.Fatal("manifest has no version for the root package")
	}

	cb, err := os.ReadFile(filepath.Join(root, "release-please-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Packages map[string]struct {
			ExtraFiles []string `json:"extra-files"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(cb, &cfg); err != nil {
		t.Fatal(err)
	}
	extra := cfg.Packages["."].ExtraFiles
	listed := map[string]bool{}
	for _, f := range extra {
		listed[f] = true
	}
	for _, must := range []string{"README.md", "llms.txt", "docs/ai-guide.md", "docs/capabilities.md", "docs/supported-versions.md"} {
		if !listed[must] {
			t.Errorf("release-please extra-files does not list %s", must)
		}
	}

	for _, f := range extra {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Errorf("extra-files entry %s: %v", f, err)
			continue
		}
		marked := 0
		for i, line := range strings.Split(string(b), "\n") {
			if !strings.Contains(line, versionMarker) {
				continue
			}
			marked++
			if !strings.Contains(line, version) {
				t.Errorf("%s:%d carries the version marker but not %s", f, i+1, version)
			}
		}
		if marked == 0 {
			t.Errorf("%s is in extra-files but has no %s marker", f, versionMarker)
		}
	}
}
