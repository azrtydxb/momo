// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

func hashedPin(name, version string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `==` + regexp.QuoteMeta(version) + ` \\\n\s+--hash=sha256:[0-9a-f]{64}`)
}

func TestDocsRequirementsPinned(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "requirements.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]string{"mkdocs": "1.6.1", "mkdocs-material": "9.8.0", "mike": "2.2.0"} {
		if !hashedPin(name, version).MatchString(string(b)) {
			t.Errorf("docs/requirements.txt does not pin %s==%s with a sha256 hash", name, version)
		}
	}
	d, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(d), "package-ecosystem: pip") || !strings.Contains(string(d), "directory: /docs") {
		t.Error("dependabot.yml has no pip entry for /docs")
	}
}
