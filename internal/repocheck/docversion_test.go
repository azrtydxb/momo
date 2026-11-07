// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	versionMarker = "x-release-please-version"
	blockStart    = "x-release-please-start-version"
	blockEnd      = "x-release-please-end"
)

// semverRe matches what release-please rewrites on a marked line.
var semverRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

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
		inBlock := false
		for i, line := range strings.Split(string(b), "\n") {
			switch {
			case strings.Contains(line, blockStart):
				inBlock = true
				marked++
				continue
			case strings.Contains(line, blockEnd):
				inBlock = false
				continue
			}
			if !inBlock && !strings.Contains(line, versionMarker) {
				continue
			}
			if !inBlock {
				marked++
			}
			found := semverRe.FindAllString(line, -1)
			if inBlock && len(found) == 0 {
				continue
			}
			// Release-please rewrites the version on a marked line; a line that
			// holds two versions would be bumped wrongly or only in part.
			if len(found) != 1 || found[0] != version {
				t.Errorf("%s:%d is marked and must hold exactly one version, %s; found %v", f, i+1, version, found)
			}
		}
		if inBlock {
			t.Errorf("%s has %s without %s", f, blockStart, blockEnd)
		}
		if marked == 0 {
			t.Errorf("%s is in extra-files but has no %s marker", f, versionMarker)
		}
	}
}

// TestDocsNameGoFloor keeps the user-facing docs in step with the go directive
// in go.mod: each must name the exact patch version, not only 1.26.
func TestDocsNameGoFloor(t *testing.T) {
	root := repoRoot(t)
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^go (\d+\.\d+\.\d+)\s*$`).FindSubmatch(mod)
	if m == nil {
		t.Fatal("go.mod has no go directive with a patch version")
	}
	floor := string(m[1])
	for _, f := range []string{"README.md", "docs/ai-guide.md", "docs/supported-versions.md"} {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if !strings.Contains(string(b), floor) {
			t.Errorf("%s does not name the go.mod floor %s", f, floor)
		}
	}
}
