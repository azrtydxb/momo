// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
)

// exampleDirs lists the examples: every directory of examples/ except the
// shared harness, examples/internal.
func exampleDirs(t *testing.T) []string {
	t.Helper()
	root := filepath.Join(repoRoot(t), "examples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "internal" {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		t.Fatal("examples/ holds no example")
	}
	return dirs
}

func readRepoFile(t *testing.T, elem ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{repoRoot(t)}, elem...)...)) // #nosec G304 -- the repository's own files
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// checkIntegrationTargetRunsEverything fails unless the integration make
// target runs every package, which is how CI discovers examples.
func checkIntegrationTargetRunsEverything(t *testing.T) {
	t.Helper()
	target := ""
	for _, line := range strings.Split(readRepoFile(t, "Makefile"), "\n") {
		if strings.Contains(line, "test -tags integration") {
			target = line
		}
	}
	if !strings.Contains(target, "./...") || strings.Contains(target, "-run") {
		t.Errorf("the integration make target must run every package without -run, or examples are not run in CI: %q", target)
	}
}

// TestExamplesAreWired fails when an example would not be run by CI. The
// convention is written down in examples/README.md: each examples/<name>/ is
// a program with a README and a test, and the test that needs a database is
// built with the integration tag, which the integration job runs for every
// package (make test-integration-postgres). A new example therefore needs no
// CI edit, and this test keeps it from being added without its test.
func TestExamplesAreWired(t *testing.T) {
	root := repoRoot(t)
	checkIntegrationTargetRunsEverything(t)

	index := readRepoFile(t, "examples", "README.md")
	docs := readRepoFile(t, "docs", "README.md")
	if !strings.Contains(docs, "examples/README.md") {
		t.Error("docs/README.md does not link examples/README.md")
	}
	for _, name := range exampleDirs(t) {
		dir := filepath.Join(root, "examples", name)
		for _, f := range []string{"main.go", "README.md"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
				t.Errorf("examples/%s has no %s", name, f)
			}
		}
		if !strings.Contains(index, "("+name+"/)") {
			t.Errorf("examples/README.md does not link examples/%s as (%s/)", name, name)
		}
		if !strings.Contains(docs, "(../examples/"+name+"/)") {
			t.Errorf("docs/README.md does not link examples/%s as (../examples/%s/)", name, name)
		}
		tests, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(tests) == 0 {
			t.Errorf("examples/%s has no test: add one that runs the example end to end", name)
		}
		for _, p := range tests {
			b, err := os.ReadFile(p) // #nosec G304 -- the repository's own files
			if err != nil {
				t.Fatal(err)
			}
			src := string(b)
			if (strings.Contains(src, "exampletest.") || strings.Contains(src, "testdb.")) && !strings.Contains(src, "//go:build integration") {
				t.Errorf("%s uses the database harness but lacks //go:build integration, so no CI job would run it with a database", filepath.Base(p))
			}
		}
	}
}

// TestExampleMigrationSumsMatch verifies, without a database, that the
// momo.sum of every example still matches its migration files. A formatter
// that rewrites a *.snapshot.json changes its checksum, so this fails in the
// unit job instead of at `momo migrate apply` time.
func TestExampleMigrationSumsMatch(t *testing.T) {
	for _, name := range exampleDirs(t) {
		dir := filepath.Join(repoRoot(t), "examples", name, "migrations")
		if _, err := os.Stat(dir); err != nil {
			continue // an example without migrations has nothing to verify
		}
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(dir, migrate.SumFileName)) // #nosec G304 -- the repository's own files
			if err != nil {
				t.Fatalf("examples/%s/migrations: %v; run `momo migrate hash` there", name, err)
			}
			defer f.Close()
			want, err := migrate.ParseSum(f)
			if err != nil {
				t.Fatalf("examples/%s/migrations/%s: %v; run `momo migrate hash` there", name, migrate.SumFileName, err)
			}
			if bad := migrate.VerifySum(os.DirFS(dir), want); len(bad) > 0 {
				t.Errorf("examples/%s/migrations: %s does not match %v. A formatter probably rewrote a snapshot; run `momo migrate hash` in examples/%s and commit the result",
					name, migrate.SumFileName, bad, name)
			}
		})
	}
}
