// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// allowedRootPrefixes are the file-prefix taxonomy of the root package (see
// .procoder/research/package-split.md, section 5, and CONTRIBUTING.md).
var allowedRootPrefixes = []string{
	"db", "tx", "driver", "err", "build", "meta", "expr", "col", "select",
	"insert", "update", "delete", "upsert", "write", "raw", "rel",
}

// sourceConforms reports whether a non-test root file name is doc.go or is a
// prefix, bare or followed by an underscore and a topic.
func sourceConforms(name string) bool {
	stem := strings.TrimSuffix(name, ".go")
	if stem == "doc" {
		return true
	}
	for _, p := range allowedRootPrefixes {
		if stem == p || strings.HasPrefix(stem, p+"_") {
			return true
		}
	}
	return false
}

// testSubject returns the subject file of a root test file: the name without
// _test and without the _internal and _integration markers (in that order, so
// _integration_internal_test.go names its real subject), plus .go.
func testSubject(name string) string {
	s := strings.TrimSuffix(strings.TrimSuffix(name, ".go"), "_test")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "_internal"), "_integration")
	return s + ".go"
}

// rootPrefixProblems checks the .go file names of the root directory against
// the taxonomy. exceptions is the ratchet list and frozen the snapshot it may
// not grow beyond. Every problem is one line.
func rootPrefixProblems(files []string, exceptions, frozen map[string]bool) []string {
	have := make(map[string]bool, len(files))
	for _, f := range files {
		have[f] = true
	}
	var out []string
	for e := range exceptions {
		if !have[e] {
			out = append(out, fmt.Sprintf("prefix_exceptions.txt lists %s, which no longer exists: remove it from the list", e))
		}
		if !frozen[e] {
			out = append(out, fmt.Sprintf("prefix_exceptions.txt lists %s, which was not a root exception when the guard was added: rename the file instead of listing it", e))
		}
	}
	for _, f := range files {
		if exceptions[f] {
			continue
		}
		if strings.HasSuffix(f, "_test.go") {
			if sub := testSubject(f); !have[sub] || strings.HasSuffix(sub, "_test.go") {
				out = append(out, fmt.Sprintf("%s: test file needs its subject %s in the root package (tests are named <subject>_test.go, <subject>_internal_test.go or <subject>_integration_test.go)", f, sub))
			}
			continue
		}
		if !sourceConforms(f) {
			out = append(out, fmt.Sprintf("%s: root files are doc.go or start with one of the prefixes %s, bare or as <prefix>_<topic>.go", f, strings.Join(allowedRootPrefixes, ", ")))
		}
	}
	sort.Strings(out)
	return out
}

func setOf(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// readNameList reads a testdata list: one file name per line, # comments.
func readNameList(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func TestRootFilePrefixes(t *testing.T) {
	entries, err := os.ReadDir(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			files = append(files, e.Name())
		}
	}
	exceptions := setOf(readNameList(t, "prefix_exceptions.txt"))
	frozen := setOf(readNameList(t, "prefix_frozen.txt"))
	for _, p := range rootPrefixProblems(files, exceptions, frozen) {
		t.Error(p)
	}
}

func TestRootFilePrefixRules(t *testing.T) {
	base := []string{"doc.go", "db.go", "expr_ops.go", "expr_ops_test.go", "builder.go", "builder_test.go"}
	frozen := setOf([]string{"builder.go", "scalars.go"})
	exc := setOf([]string{"builder.go"})

	t.Run("conforming files pass", func(t *testing.T) {
		if got := rootPrefixProblems(base, exc, frozen); len(got) != 0 {
			t.Fatalf("unexpected problems: %v", got)
		}
	})
	t.Run("new file with a foreign prefix fails and lists the prefixes", func(t *testing.T) {
		got := rootPrefixProblems(append([]string{"zzz_probe.go"}, base...), exc, frozen)
		if len(got) != 1 || !strings.Contains(got[0], "zzz_probe.go") {
			t.Fatalf("want one problem naming zzz_probe.go, got %v", got)
		}
		for _, p := range allowedRootPrefixes {
			if !strings.Contains(got[0], p) {
				t.Errorf("message does not list prefix %q: %s", p, got[0])
			}
		}
	})
	t.Run("a prefix must end at an underscore", func(t *testing.T) {
		if got := rootPrefixProblems(append([]string{"dbx.go", "selection.go"}, base...), exc, frozen); len(got) != 2 {
			t.Fatalf("want 2 problems, got %v", got)
		}
	})
	t.Run("stale exception fails", func(t *testing.T) {
		stale := setOf([]string{"builder.go", "scalars.go"})
		got := rootPrefixProblems(base, stale, frozen)
		if len(got) != 1 || !strings.Contains(got[0], "scalars.go") || !strings.Contains(got[0], "no longer exists") {
			t.Fatalf("want a stale-entry problem for scalars.go, got %v", got)
		}
	})
	t.Run("exception outside the frozen snapshot fails", func(t *testing.T) {
		files := append([]string{"zzz_probe.go"}, base...)
		got := rootPrefixProblems(files, setOf([]string{"builder.go", "zzz_probe.go"}), frozen)
		if len(got) != 1 || !strings.Contains(got[0], "zzz_probe.go") || !strings.Contains(got[0], "not a root exception") {
			t.Fatalf("want a not-frozen problem for zzz_probe.go, got %v", got)
		}
	})
	t.Run("test file needs its subject", func(t *testing.T) {
		files := append([]string{"orphan_integration_test.go", "db_internal_test.go"}, base...)
		got := rootPrefixProblems(files, exc, frozen)
		if len(got) != 1 || !strings.Contains(got[0], "orphan_integration_test.go") || !strings.Contains(got[0], "orphan.go") {
			t.Fatalf("want one problem for orphan_integration_test.go, got %v", got)
		}
	})
	t.Run("integration_internal test file maps to the bare subject", func(t *testing.T) {
		ok := append([]string{"db_integration_internal_test.go"}, base...)
		if got := rootPrefixProblems(ok, exc, frozen); len(got) != 0 {
			t.Fatalf("unexpected problems: %v", got)
		}
		orphan := append([]string{"orphan_integration_internal_test.go"}, base...)
		got := rootPrefixProblems(orphan, exc, frozen)
		if len(got) != 1 || !strings.Contains(got[0], "needs its subject orphan.go") {
			t.Fatalf("want a problem naming subject orphan.go, got %v", got)
		}
	})
	t.Run("an exception covers a test file without subject", func(t *testing.T) {
		files := append([]string{"orphan_test.go"}, base...)
		both := setOf([]string{"builder.go", "orphan_test.go"})
		if got := rootPrefixProblems(files, both, both); len(got) != 0 {
			t.Fatalf("unexpected problems: %v", got)
		}
	})
}
