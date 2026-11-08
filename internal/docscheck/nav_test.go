// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minNavPages is the floor for the navigation: the pages that exist today.
const minNavPages = 14

func TestMkdocsNavPagesExist(t *testing.T) {
	root := repoRoot(t)
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < minNavPages {
		t.Fatalf("nav lists %d pages, want at least %d", len(pages), minNavPages)
	}
	for _, p := range pages {
		if _, err := os.Stat(filepath.Join(root, "docs", p)); err != nil {
			t.Errorf("nav page %s: %v", p, err)
		}
	}
}

func TestEveryDocPageInNav(t *testing.T) {
	root := repoRoot(t)
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	inNav := map[string]bool{}
	for _, p := range pages {
		inNav[filepath.ToSlash(p)] = true
	}
	docs := filepath.Join(root, "docs")
	err = filepath.WalkDir(docs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, err := filepath.Rel(docs, path)
		if err != nil {
			return err
		}
		if !inNav[filepath.ToSlash(rel)] {
			t.Errorf("docs/%s is not in the mkdocs.yml nav: add it under nav in mkdocs.yml and link it from docs/README.md", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
