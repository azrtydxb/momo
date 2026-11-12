// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// Catches: a path that git on Linux or macOS happily holds but a Windows
// checkout cannot create, which fails the Windows job only (an agent once
// committed a watcher log named "<branch>:<sha>" at the repo root, and the
// branch looked green because ci-agent has no Windows job). Also catches two
// paths that differ only by case, which collapse into one file on Windows and
// macOS.

// windowsReserved are the device names Windows refuses as a file name, with or
// without an extension.
var windowsReserved = func() map[string]bool {
	m := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	for _, d := range "123456789" {
		m["COM"+string(d)] = true
		m["LPT"+string(d)] = true
	}
	return m
}()

// componentProblem returns why one path component cannot exist on Windows, or
// "" when it can.
func componentProblem(name string) string {
	for _, r := range name {
		if strings.ContainsRune(`:*?"<>|\`, r) {
			return "contains " + string(r)
		}
		if unicode.IsControl(r) {
			return "contains a control character"
		}
	}
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return "ends in a space or a dot"
	}
	stem, _, _ := strings.Cut(name, ".")
	if windowsReserved[strings.ToUpper(strings.TrimRight(stem, " "))] {
		return "is a Windows reserved name"
	}
	return ""
}

// fileNameProblems checks slash-separated repo-relative paths. Every problem
// is one line starting with the offending path.
func fileNameProblems(paths []string) []string {
	var out []string
	folded := make(map[string]string, len(paths))
	for _, p := range paths {
		for _, c := range strings.Split(p, "/") {
			if why := componentProblem(c); why != "" {
				out = append(out, p+": "+why+" (not creatable on a Windows checkout)")
				break
			}
		}
		key := strings.ToLower(p)
		if other, ok := folded[key]; ok && other != p {
			out = append(out, p+": differs from "+other+" only by case (one file on Windows and macOS)")
		}
		folded[key] = p
	}
	sort.Strings(out)
	return out
}

func TestRepoFileNamesAreWindowsSafe(t *testing.T) {
	root := repoRoot(t)
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // a scratch directory another package's test removed mid-walk
			}
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "vendor":
				return filepath.SkipDir
			}
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel != "." {
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range fileNameProblems(paths) {
		t.Error(p)
	}
}

func TestFileNameRules(t *testing.T) {
	bad := []string{
		"worktree-agent-adffe7ae4d2ad2c57:61fe0ce", "a*b", "a?b", `a"b`, "a<b", "a>b", "a|b", `a\b`,
		"dir/tab\there", "trailing ", "trailing.", "dir./x", "CON", "con.txt", "Nul.tar.gz", "com1", "LPT9.log", "aux ",
	}
	for _, p := range bad {
		if got := fileNameProblems([]string{p}); len(got) != 1 || !strings.HasPrefix(got[0], p+":") {
			t.Errorf("%q: want one problem naming the path, got %v", p, got)
		}
	}
	ok := []string{"README.md", "internal/repocheck/x_test.go", ".github/workflows/ci.yml", "console.go", "com10.txt", "lpt0", "a b.txt", "a.b.c"}
	if got := fileNameProblems(ok); len(got) != 0 {
		t.Errorf("valid names flagged: %v", got)
	}
	got := fileNameProblems([]string{"docs/Readme.md", "docs/README.md", "other"})
	if len(got) != 1 || !strings.Contains(got[0], "only by case") {
		t.Errorf("want one case-collision problem, got %v", got)
	}
}
