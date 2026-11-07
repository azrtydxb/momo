// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// walkTestFiles calls fn with the slash path (relative to the repository root)
// and the content of every _test.go file of the root module. Directories with
// their own go.mod are separate modules and are skipped.
func walkTestFiles(t *testing.T, fn func(rel, content string)) {
	t.Helper()
	root := repoRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules":
				return filepath.SkipDir
			}
			if path != root {
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fn(filepath.ToSlash(rel), string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var (
	// dbHelperImport matches an import of a package that hands out databases.
	dbHelperImport = regexp.MustCompile(`"github\.com/azrtydxb/momo/(internal/testdb|internal/testdb/testopen|examples/internal/exampletest)"`)
	// dbHelperUse matches a use of the relfixture helpers that open a database;
	// the rest of relfixture (Hook, Counting, WithoutBatch) needs none.
	dbHelperUse = regexp.MustCompile(`\brelfixture\.(Each|SeedBasic|CreateSchema|PostgresDriver)\b`)
	buildTag    = regexp.MustCompile(`(?m)^//go:build .*\bintegration\b`)
	packageLine = regexp.MustCompile(`(?m)^package `)
)

// hasIntegrationTag reports whether the build constraint comes before the
// package clause, which is where Go reads it.
func hasIntegrationTag(content string) bool {
	loc := packageLine.FindStringIndex(content)
	if loc == nil {
		return false
	}
	return buildTag.MatchString(content[:loc[0]])
}

// TestIntegrationTagConvention enforces one convention for database tests: a
// test file that opens a database (through testdb, testopen, exampletest,
// relfixture.Each and friends, or a MOMO_TEST_ variable) carries the
// integration build tag. Untagged builds, which is what make test and the
// macOS and Windows jobs run, then contain no test that could skip silently.
func TestIntegrationTagConvention(t *testing.T) {
	walkTestFiles(t, func(rel, content string) {
		if rel == "internal/repocheck/testconv_test.go" {
			return
		}
		usesDB := dbHelperImport.MatchString(content) || dbHelperUse.MatchString(content) ||
			strings.Contains(content, "MOMO_TEST_")
		if usesDB && !hasIntegrationTag(content) {
			t.Errorf("%s opens a test database but has no //go:build integration tag", rel)
		}
	})
}

var fuzzFunc = regexp.MustCompile(`(?m)^func (Fuzz\w+)\(\w+ \*testing\.F\)`)

// TestFuzzTargetsListed fails when a Fuzz function is not in the Makefile
// FUZZ_TARGETS list, because make test-fuzz is what runs it in CI, and when the
// list names a function that does not exist.
func TestFuzzTargetsListed(t *testing.T) {
	var found []string
	walkTestFiles(t, func(rel, content string) {
		dir := filepath.ToSlash(filepath.Dir(rel))
		pkg := "./" + dir
		if dir == "." {
			pkg = "."
		}
		for _, m := range fuzzFunc.FindAllStringSubmatch(content, -1) {
			found = append(found, pkg+":"+m[1])
		}
	})

	mk, err := os.ReadFile(filepath.Join(repoRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	listed := regexp.MustCompile(`(?m)^FUZZ_TARGETS\s*\+?=\s*(\S+)\s*$`).FindAllStringSubmatch(string(mk), -1)
	var want []string
	for _, m := range listed {
		want = append(want, m[1])
	}
	slices.Sort(found)
	slices.Sort(want)
	if len(found) == 0 {
		t.Fatal("found no Fuzz functions; the walk is broken")
	}
	for _, f := range found {
		if !slices.Contains(want, f) {
			t.Errorf("fuzz target %s is not in FUZZ_TARGETS in the Makefile", f)
		}
	}
	for _, w := range want {
		if !slices.Contains(found, w) {
			t.Errorf("FUZZ_TARGETS lists %s, which is not a Fuzz function in the tree", w)
		}
	}
}
