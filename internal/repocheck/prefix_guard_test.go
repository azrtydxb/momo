// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"fmt"
	"os"
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

// stemConforms reports whether a file stem is doc or is a prefix, bare or
// followed by an underscore and a topic.
func stemConforms(stem string) bool {
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

// rootPrefixProblems checks the .go file names of the root directory against
// the taxonomy. A source file must be doc.go or start with an allowed prefix.
// A test file follows its subject, so its name without the _test suffix must
// start with an allowed prefix too (foo_test.go, foo_internal_test.go and
// foo_integration_test.go for a subject foo.go). Every problem is one line.
func rootPrefixProblems(files []string) []string {
	var out []string
	for _, f := range files {
		stem := strings.TrimSuffix(strings.TrimSuffix(f, ".go"), "_test")
		if stemConforms(stem) {
			continue
		}
		what := "root files are doc.go or"
		if strings.HasSuffix(f, "_test.go") {
			what = "root test files are named after their subject file, which"
		}
		out = append(out, fmt.Sprintf("%s: %s must start with one of the prefixes %s, bare or as <prefix>_<topic>.go (see CONTRIBUTING.md, Root package file names)", f, what, strings.Join(allowedRootPrefixes, ", ")))
	}
	sort.Strings(out)
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
	if len(files) == 0 {
		t.Fatal("no .go files found in the repository root; the guard would pass without checking anything")
	}
	for _, p := range rootPrefixProblems(files) {
		t.Error(p)
	}
}

func TestRootFilePrefixRules(t *testing.T) {
	base := []string{"doc.go", "db.go", "expr_ops.go", "expr_ops_test.go", "build.go", "build_test.go", "db_integration_internal_test.go"}

	t.Run("conforming files pass", func(t *testing.T) {
		if got := rootPrefixProblems(base); len(got) != 0 {
			t.Fatalf("unexpected problems: %v", got)
		}
	})
	t.Run("new file with a foreign prefix fails and lists the prefixes", func(t *testing.T) {
		got := rootPrefixProblems(append([]string{"zzz_probe.go"}, base...))
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
		if got := rootPrefixProblems(append([]string{"dbx.go", "selection.go"}, base...)); len(got) != 2 {
			t.Fatalf("want 2 problems, got %v", got)
		}
	})
	t.Run("a test file with a foreign prefix fails", func(t *testing.T) {
		got := rootPrefixProblems(append([]string{"orphan_integration_test.go", "zzz_test.go"}, base...))
		if len(got) != 2 || !strings.Contains(got[0], "orphan_integration_test.go") || !strings.Contains(got[1], "zzz_test.go") {
			t.Fatalf("want problems for orphan_integration_test.go and zzz_test.go, got %v", got)
		}
	})
	t.Run("the old unprefixed names fail", func(t *testing.T) {
		old := []string{"errors.go", "builder.go", "scalars.go", "errors_test.go"}
		if got := rootPrefixProblems(old); len(got) != len(old) {
			t.Fatalf("want %d problems, got %v", len(old), got)
		}
	})
}
