// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Architecture roles read from testdata/packages.txt.
const (
	roleCore     = "core"
	roleLeaf     = "leaf"
	roleFeature  = "feature"
	roleIntLeaf  = "internal-leaf"
	roleIntFixt  = "internal-fixture"
	roleOther    = "other"
	packagesFile = "internal/repocheck/testdata/packages.txt"
)

type pkgEntry struct{ role, path string }

// parsePackages reads the role table: "<role> <path>" per line, "#" comments.
func parsePackages(text string) ([]pkgEntry, error) {
	var out []pkgEntry
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			return nil, fmt.Errorf("%s:%d: want \"<role> <path>\", got %q", packagesFile, i+1, line)
		}
		switch f[0] {
		case roleCore, roleLeaf, roleFeature, roleIntLeaf, roleIntFixt, roleOther:
		default:
			return nil, fmt.Errorf("%s:%d: unknown role %q", packagesFile, i+1, f[0])
		}
		out = append(out, pkgEntry{f[0], f[1]})
	}
	return out, nil
}

func loadPackages(t *testing.T) []pkgEntry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "repocheck", "testdata", "packages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := parsePackages(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// roleOf returns the role of the module-relative directory rel ("." is the root).
func roleOf(entries []pkgEntry, rel string) (string, bool) {
	for _, e := range entries {
		if e.path == rel {
			return e.role, true
		}
		if p, ok := strings.CutSuffix(e.path, "/..."); ok && (rel == p || strings.HasPrefix(rel, p+"/")) {
			return e.role, true
		}
	}
	return "", false
}

func relImport(imp string) string {
	if imp == modPath {
		return "."
	}
	return strings.TrimPrefix(imp, modPath+"/")
}

// internalRootViolations reports every internal-leaf package that depends on
// the root, and every internal package the root itself depends on that is not
// an internal-leaf. graph maps an import path to its transitive dependencies.
func internalRootViolations(entries []pkgEntry, graph map[string][]string) []string {
	var out []string
	for pkg, deps := range graph {
		if !strings.HasPrefix(pkg, modPath+"/internal/") {
			continue
		}
		if role, _ := roleOf(entries, relImport(pkg)); role != roleIntLeaf {
			continue
		}
		for _, d := range deps {
			if d == modPath {
				out = append(out, pkg+" depends on "+modPath+"; internal packages the root can use must not import it back")
			}
		}
	}
	for _, d := range graph[modPath] {
		if !strings.HasPrefix(d, modPath+"/internal/") {
			continue
		}
		if role, _ := roleOf(entries, relImport(d)); role != roleIntLeaf {
			out = append(out, modPath+" depends on "+d+", which is not listed as internal-leaf in "+packagesFile)
		}
	}
	sort.Strings(out)
	return out
}

// featureViolations reports module packages a feature package imports beyond
// the root, the leaf set and internal/*.
func featureViolations(feature string, deps []string) []string {
	var out []string
	for _, d := range deps {
		if d == feature || coreAllowed[d] || strings.HasPrefix(d, modPath+"/internal/") || !strings.HasPrefix(d, modPath) {
			continue
		}
		out = append(out, feature+" depends on "+d+"; a feature package may import only momo, dialect, types, vector, schema and internal/*")
	}
	return out
}

// unknownDirs reports directories (module-relative, slash-separated) not
// covered by the role table.
func unknownDirs(entries []pkgEntry, dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if _, ok := roleOf(entries, d); !ok {
			out = append(out, d)
		}
	}
	return out
}

func goDeps(t *testing.T, patterns ...string) map[string][]string {
	t.Helper()
	args := append([]string{"list", "-e", "-f", "{{.ImportPath}} {{join .Deps \" \"}}"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	graph := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 {
			graph[f[0]] = f[1:]
		}
	}
	return graph
}

// TestInternalNeverImportsRoot keeps the dependency arrow one-way: the
// internal packages the root builds on never import the root. Nested fixture
// modules are not part of ./internal/... and are skipped by go list.
func TestInternalNeverImportsRoot(t *testing.T) {
	entries := loadPackages(t)
	graph := goDeps(t, ".", "./internal/...")
	for _, v := range internalRootViolations(entries, graph) {
		t.Error(v)
	}
}

// featurePatterns returns the import paths of the feature entries.
func featurePatterns(entries []pkgEntry) []string {
	var out []string
	for _, e := range entries {
		if e.role == roleFeature {
			out = append(out, modPath+"/"+e.path)
		}
	}
	return out
}

// featureGraphViolations checks every feature pattern against the dependency
// graph: a pattern go list did not find is a violation, as is a forbidden import.
func featureGraphViolations(patterns []string, graph map[string][]string) []string {
	var out []string
	for _, p := range patterns {
		deps, ok := graph[p]
		if !ok {
			out = append(out, p+" is listed as a feature in "+packagesFile+" but go list does not find it")
			continue
		}
		out = append(out, featureViolations(p, deps)...)
	}
	return out
}

// staleEntries reports non-wildcard entries whose directory does not exist,
// so a deleted or renamed package cannot linger in the table.
func staleEntries(entries []pkgEntry, exists func(rel string) bool) []string {
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.path, "/...") {
			continue
		}
		if !exists(e.path) {
			out = append(out, e.path)
		}
	}
	return out
}

// TestFeaturePackagesImportOnlyAllowed pins what the feature subpackages
// listed in testdata/packages.txt may import. With no feature line the test is
// skipped, visibly; the first feature line makes it run go list for real.
func TestFeaturePackagesImportOnlyAllowed(t *testing.T) {
	patterns := featurePatterns(loadPackages(t))
	if len(patterns) == 0 {
		t.Skipf("no feature packages in %s yet; the check runs once one is listed", packagesFile)
	}
	for _, v := range featureGraphViolations(patterns, goDeps(t, patterns...)) {
		t.Error(v)
	}
}

// TestKnownPackages forces a deliberate decision for every new package.
func TestKnownPackages(t *testing.T) {
	root := repoRoot(t)
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // a scratch directory another package's test removed mid-walk
			}
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir // a nested module is not part of this one
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") {
			rel, err := filepath.Rel(root, filepath.Dir(p))
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if len(dirs) == 0 || dirs[len(dirs)-1] != rel {
				dirs = append(dirs, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := loadPackages(t)
	for _, d := range unknownDirs(entries, dirs) {
		t.Errorf("directory %q has Go files but is not listed in %s; add a line with its role", d, packagesFile)
	}
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
	for _, d := range staleEntries(entries, exists) {
		t.Errorf("%s lists %q, which is not a directory; remove or rename the line", packagesFile, d)
	}
}

// TestArchitectureRulesCatchViolations plants a violation in each rule, so the
// checks cannot pass by checking nothing.
func TestArchitectureRulesCatchViolations(t *testing.T) {
	entries, err := parsePackages("core .\nleaf types\nfeature fts\ninternal-leaf internal/chunk\ninternal-fixture internal/suite\nother cmd/...\n")
	if err != nil {
		t.Fatal(err)
	}
	graph := map[string][]string{
		modPath:                       {modPath + "/internal/chunk", modPath + "/internal/suite"},
		modPath + "/internal/chunk":   {modPath},
		modPath + "/internal/suite":   {modPath},
		modPath + "/internal/missing": {modPath},
	}
	got := strings.Join(internalRootViolations(entries, graph), "\n")
	for _, want := range []string{"internal/chunk depends on", "depends on " + modPath + "/internal/suite, which is not listed"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing violation %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "internal/missing") {
		t.Errorf("an unlisted internal package is TestKnownPackages' job, not this rule's:\n%s", got)
	}

	if v := featureViolations(modPath+"/fts", []string{modPath + "/fts", modPath, modPath + "/types", modPath + "/internal/redact", "example.com/other/dep"}); len(v) != 0 {
		t.Errorf("allowed imports flagged: %v", v)
	}
	if v := featureViolations(modPath+"/fts", []string{modPath + "/jsonop"}); len(v) != 1 {
		t.Errorf("a feature importing another feature must be flagged, got %v", v)
	}
	if v := featureViolations(modPath+"/fts", []string{modPath + "/migrate"}); len(v) != 1 {
		t.Errorf("a feature importing migrate must be flagged, got %v", v)
	}

	if u := unknownDirs(entries, []string{".", "cmd/momo/x", "newpkg", "internal/suite/sub"}); len(u) != 2 || u[0] != "newpkg" {
		t.Errorf("unknown directories = %v, want [newpkg internal/suite/sub]", u)
	}
	if st := staleEntries(entries, func(rel string) bool { return rel != "fts" && rel != "internal/chunk" }); len(st) != 2 || st[0] != "fts" || st[1] != "internal/chunk" {
		t.Errorf("stale entries = %v, want [fts internal/chunk] (cmd/... is a wildcard and is skipped)", st)
	}
	patterns := featurePatterns(entries)
	if len(patterns) != 1 || patterns[0] != modPath+"/fts" {
		t.Fatalf("feature patterns = %v, want the fts feature", patterns)
	}
	if v := featureGraphViolations(patterns, map[string][]string{}); len(v) != 1 || !strings.Contains(v[0], "go list does not find it") {
		t.Errorf("a feature go list cannot find must be flagged, got %v", v)
	}
	if v := featureGraphViolations(patterns, map[string][]string{modPath + "/fts": {modPath + "/jsonop"}}); len(v) != 1 {
		t.Errorf("a feature importing another feature must be flagged through the graph, got %v", v)
	}
	if _, err := parsePackages("bogus x\n"); err == nil {
		t.Error("an unknown role must be rejected")
	}
}
