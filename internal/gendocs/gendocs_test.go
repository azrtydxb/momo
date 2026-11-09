// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Catches: a code change (a dialect flag, a typemap entry, a driver package, a
// go.mod version) merged without running go generate, so a doc block states
// something the code no longer does.
func TestGeneratedBlocksAreCurrent(t *testing.T) {
	stale, err := Stale(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stale {
		t.Errorf("%s is out of date: run go generate ./internal/gendocs", s)
	}
}

// Catches: a generator whose output changes between runs (map iteration,
// timestamps), which would make the CI drift check flaky.
func TestRenderIsDeterministic(t *testing.T) {
	root := repoRoot(t)
	for _, tg := range Targets {
		for _, name := range tg.Blocks {
			a, err := Render(root, name)
			if err != nil {
				t.Fatal(err)
			}
			b, err := Render(root, name)
			if err != nil {
				t.Fatal(err)
			}
			if a != b {
				t.Errorf("block %s differs between two renders", name)
			}
			doc := "before\n" + strings.Join([]string{"<!-- generated:" + name + " -->", "old", "<!-- /generated:" + name + " -->"}, "\n") + "\nafter\n"
			once, err := Splice(doc, name, a)
			if err != nil {
				t.Fatal(err)
			}
			twice, err := Splice(once, name, a)
			if err != nil {
				t.Fatal(err)
			}
			if once != twice {
				t.Errorf("splicing block %s twice changes the document", name)
			}
			if !strings.HasPrefix(once, "before\n") || !strings.HasSuffix(once, "\nafter\n") {
				t.Errorf("splicing block %s touched the prose around it", name)
			}
			got, err := Extract(once, name)
			if err != nil || got != a {
				t.Errorf("Extract after Splice of %s = %q, %v", name, got, err)
			}
		}
	}
}

// Catches: a malformed marker pair being silently accepted and the block
// written in the wrong place.
func TestSpliceRejectsBadMarkers(t *testing.T) {
	b, e := Markers("x")
	for name, doc := range map[string]string{
		"missing":  "text\n",
		"no end":   b + "\n",
		"reversed": e + "\n" + b + "\n",
		"twice":    b + "\n" + e + "\n" + b + "\n" + e + "\n",
	} {
		if _, err := Splice(doc, "x", "body"); err == nil {
			t.Errorf("%s: Splice accepted %q", name, doc)
		}
	}
}

var depRow = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|\\s*(\\S+)\\s*\\|")

// Catches: a dependency row in docs/supported-versions.md that differs from
// go.mod, or a direct requirement with no row. It reads the doc and go.mod
// directly, not through the generator.
func TestDependencyRowsMatchGoMod(t *testing.T) {
	root := repoRoot(t)
	gomod, err := readFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := readFile(filepath.Join(root, "docs", "supported-versions.md"))
	if err != nil {
		t.Fatal(err)
	}
	block, err := Extract(doc, "dependencies")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		if m := depRow.FindStringSubmatch(line); m != nil {
			rows[m[1]] = m[2]
		}
	}
	want := DirectDeps(gomod)
	if len(want) == 0 {
		t.Fatal("go.mod has no direct requirements; the parser is stale")
	}
	for _, d := range want {
		if got, ok := rows[d[0]]; !ok {
			t.Errorf("docs/supported-versions.md has no row for %s", d[0])
		} else if got != d[1] {
			t.Errorf("docs/supported-versions.md says %s %s, go.mod requires %s", d[0], got, d[1])
		}
		delete(rows, d[0])
	}
	for m := range rows {
		t.Errorf("docs/supported-versions.md lists %s, which go.mod does not require directly", m)
	}
}

// Catches: the type table in docs/TYPE_MAPPING.md drifting from gen/typemap:
// the block must equal the generator's output, and it must name every Go type
// of the generator's list with one column per dialect.
func TestTypeTableMatchesTypemap(t *testing.T) {
	root := repoRoot(t)
	doc, err := readFile(filepath.Join(root, "docs", "TYPE_MAPPING.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Extract(doc, "typemap")
	if err != nil {
		t.Fatal(err)
	}
	want, err := typemapBlock()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("docs/TYPE_MAPPING.md typemap block differs from the typemap output:\n--- doc\n%s\n--- generated\n%s", got, want)
	}
	if !strings.Contains(want, "| `int` ") || !strings.Contains(want, "unsupported") {
		t.Errorf("type table lacks the int row or an unsupported cell:\n%s", want)
	}
	if hdr := strings.SplitN(want, "\n", 2)[0]; !strings.Contains(hdr, "PostgreSQL") || !strings.Contains(hdr, "SQLite") || !strings.Contains(hdr, "MySQL") {
		t.Errorf("type table header %q lacks a dialect column", hdr)
	}
}

var statusProse = regexp.MustCompile(`(?i)\b(not usable|unusable|is usable|are usable)\b`)

// Catches: hand-written dialect-status prose creeping back next to the
// generated block, where it goes stale when a driver is added.
func TestNoStatusProseOutsideBlocks(t *testing.T) {
	root := repoRoot(t)
	for _, f := range []string{"README.md", "llms.txt", "docs/ai-guide.md", "docs/supported-versions.md", "docs/migrations.md", "docs/capabilities.md"} {
		doc, err := readFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(doc, "<!-- generated:status -->") {
			t.Errorf("%s has no generated status block", f)
		}
		for i, line := range strings.Split(StripBlocks(doc), "\n") {
			if statusProse.MatchString(line) {
				t.Errorf("%s: hand-written status prose outside a generated block: %q (line %d of the text without blocks)", f, line, i+1)
			}
		}
	}
}

func fakeRepo(t *testing.T, sqliteDriver bool) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "driver/pgx/d.go", "package pgx\n\nimport \"github.com/azrtydxb/momo/dialect\"\n\nvar _ = dialect.Postgres\n")
	write(t, root, "driver/my/d.go", "package my\n\nimport \"github.com/azrtydxb/momo/dialect\"\n\nvar _ = dialect.MySQLFlavor\n")
	write(t, root, "migrate/postgres/d.go", "package postgres\n")
	write(t, root, "migrate/sqlite/d.go", "package sqlite\n")
	write(t, root, "cmd/momo/m.go", "package main\n\nimport _ \"github.com/azrtydxb/momo/migrate/postgres\"\n")
	if sqliteDriver {
		write(t, root, "driver/lite/d.go", "//go:build cgo && !windows\n\npackage lite\n\nimport (\n\t\"github.com/azrtydxb/momo\"\n\t\"github.com/azrtydxb/momo/dialect\"\n)\n\nvar _ = dialect.SQLite\nvar _ = momo.Capabilities{Vectors: true}\n")
	}
	return root
}

// Catches: the usable flag not following the driver packages, which would
// leave every page saying a database is not usable after its driver ships.
func TestUsableFollowsDriverPackages(t *testing.T) {
	without, err := statusBlock(fakeRepo(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(without, "- SQLite: not usable") {
		t.Errorf("no sqlite driver, want SQLite not usable:\n%s", without)
	}
	if !strings.Contains(without, "- PostgreSQL: usable through `driver/pgx`") || !strings.Contains(without, "`migrate/postgres`, run by `momo migrate`") {
		t.Errorf("postgres line wrong:\n%s", without)
	}
	with, err := statusBlock(fakeRepo(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with, "- SQLite: usable through `driver/lite` (needs cgo, not on Windows, vector builders: yes)") {
		t.Errorf("sqlite driver added, want SQLite usable:\n%s", with)
	}
	if !strings.Contains(with, "`migrate/sqlite`, library only") {
		t.Errorf("sqlite migration line wrong:\n%s", with)
	}
}

// Catches: the feature table naming an identifier the code no longer has (a
// package move or a rename), so the docs keep the old name.
func TestFeatureIdentifiersExist(t *testing.T) {
	root := repoRoot(t)
	if err := checkFeatureAPI(root); err != nil {
		t.Fatal(err)
	}
	saved := features
	defer func() { features = saved }()
	features = append(append([]feature{}, saved...), feature{Name: "bogus", API: []string{"jsonop.NoSuchFunction"}})
	if err := checkFeatureAPI(root); err == nil {
		t.Error("checkFeatureAPI accepted an identifier jsonop does not declare")
	}
}

// Catches: the generator growing a dependency on a database driver or a cgo
// package, which would stop it running on the lint job's Go setup.
func TestGeneratorNeedsNoCgoOrDatabase(t *testing.T) {
	root := repoRoot(t)
	for _, pkg := range []string{"./internal/gendocs", "./internal/gendocs/cmd/gendocs"} {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}} {{.Standard}} {{len .CgoFiles}}", pkg) // #nosec G204 -- pkg comes from the fixed list above
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Fields(line)
			if len(f) != 3 {
				t.Fatalf("unexpected go list line %q", line)
			}
			path, std, cgo := f[0], f[1] == "true", f[2] != "0"
			for _, bad := range []string{"/driver/", "jackc/pgx", "go-sqlite3", "sqlite-vec", "go-sql-driver", "modernc.org/sqlite", "lib/pq"} {
				if strings.Contains(path, bad) {
					t.Errorf("%s depends on %s, a database package", pkg, path)
				}
			}
			if cgo && !std {
				t.Errorf("%s depends on %s, which has cgo files", pkg, path)
			}
		}
	}
}
