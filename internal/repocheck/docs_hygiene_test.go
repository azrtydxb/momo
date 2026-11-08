// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docPages returns the Markdown files directly under docs/.
func docPages(t *testing.T) []string {
	t.Helper()
	pages, err := filepath.Glob(filepath.Join(repoRoot(t), "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("docs/ holds no Markdown page")
	}
	return pages
}

var (
	planTag       = regexp.MustCompile(`\[momo-\d+`)
	procoderLink  = regexp.MustCompile(`\.procoder/`)
	indexLinkRe   = regexp.MustCompile(`\]\(([^)#\s]+\.md)(?:#[^)]*)?\)`)
	exampleLinkRe = regexp.MustCompile(`\]\(\.\./examples/([^)/#\s]+)/?[^)]*\)`)
)

// TestDocsHaveNoPlanBookkeeping keeps planning vocabulary out of the pages a
// reader sees: a plan tag such as [momo-05] or a reference into .procoder/
// means the page describes the plan instead of the code.
func TestDocsHaveNoPlanBookkeeping(t *testing.T) {
	for _, p := range docPages(t) {
		b, err := os.ReadFile(p) // #nosec G304 -- files under the repository's docs/
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if planTag.MatchString(line) {
				t.Errorf("%s:%d carries a plan tag ([momo-NN]); describe the code, not the plan", filepath.Base(p), i+1)
			}
			if procoderLink.MatchString(line) {
				t.Errorf("%s:%d refers to .procoder/, which readers of the docs cannot open", filepath.Base(p), i+1)
			}
		}
	}
}

// TestArchitecturePageDescribesCode keeps the plan sections out of
// docs/ARCHITECTURE.md: the page lists the packages that exist, not a roadmap.
func TestArchitecturePageDescribesCode(t *testing.T) {
	text := readRepoFile(t, "docs", "ARCHITECTURE.md")
	for _, banned := range []string{"Planned, not implemented", "Delivery order", "Ownership decisions"} {
		if strings.Contains(text, banned) {
			t.Errorf("docs/ARCHITECTURE.md contains %q; plan content belongs under .procoder/, not in the docs", banned)
		}
	}
}

// TestDocsIndexListsEveryPage fails when docs/README.md does not link a page
// of docs/ or links a page that does not exist, and when an example it names
// is missing under examples/.
func TestDocsIndexListsEveryPage(t *testing.T) {
	root := repoRoot(t)
	idx := readRepoFile(t, "docs", "README.md")
	linked := map[string]bool{}
	for _, m := range indexLinkRe.FindAllStringSubmatch(idx, -1) {
		if strings.Contains(m[1], "/") {
			continue // a link out of docs/, such as the examples index
		}
		linked[m[1]] = true
	}
	onDisk := map[string]bool{}
	for _, p := range docPages(t) {
		name := filepath.Base(p)
		onDisk[name] = true
		if name != "README.md" && !linked[name] {
			t.Errorf("docs/README.md does not link docs/%s", name)
		}
	}
	for name := range linked {
		if !onDisk[name] {
			t.Errorf("docs/README.md links docs/%s, which does not exist", name)
		}
	}
	for _, m := range exampleLinkRe.FindAllStringSubmatch(idx, -1) {
		if m[1] == "README.md" {
			continue
		}
		if st, err := os.Stat(filepath.Join(root, "examples", m[1])); err != nil || !st.IsDir() {
			t.Errorf("docs/README.md names the example %q, which is not a directory under examples/", m[1])
		}
	}
}
