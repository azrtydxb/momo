// SPDX-License-Identifier: Apache-2.0

//go:build docs

package docscheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var snippetLine = regexp.MustCompile(`(?m)^--8<--\s+"([^"]+)"\s*$`)

// expandSnippets mirrors the hook: an include line becomes the named file.
func expandSnippets(t *testing.T, root string, src []byte) string {
	t.Helper()
	return snippetLine.ReplaceAllStringFunc(string(src), func(line string) string {
		name := snippetLine.FindStringSubmatch(line)[1]
		b, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- the repository's own files
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimRight(string(b), "\n")
	})
}

// TestLLMsTxt checks the built site: llms.txt lists every nav page,
// llms-full.txt carries every page's Markdown, and each page has a raw .md twin.
// MOMO_DOCS_SITE names the directory mkdocs built into.
func TestLLMsTxt(t *testing.T) {
	root := repoRoot(t)
	site := os.Getenv("MOMO_DOCS_SITE")
	if site == "" {
		t.Fatal("MOMO_DOCS_SITE must point at the built site directory")
	}
	llms, err := os.ReadFile(filepath.Join(site, "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	full, err := os.ReadFile(filepath.Join(site, "llms-full.txt"))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if !strings.Contains(string(llms), "("+p+")") {
			t.Errorf("llms.txt does not list %s", p)
		}
		raw, err := os.ReadFile(filepath.Join(root, "docs", p))
		if err != nil {
			t.Fatal(err)
		}
		src := expandSnippets(t, root, raw)
		if !strings.Contains(string(full), "# Source: "+p+"\n") || !strings.Contains(string(full), src) {
			t.Errorf("llms-full.txt lacks the Markdown of %s", p)
		}
		twin, err := os.ReadFile(filepath.Join(site, p))
		if err != nil {
			t.Errorf("no raw Markdown twin for %s: %v", p, err)
			continue
		}
		if string(twin) != src {
			t.Errorf("raw Markdown twin of %s differs from its source", p)
		}
	}
}
