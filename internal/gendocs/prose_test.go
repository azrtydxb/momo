// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// statusProse matches hand-written statements about which databases are usable
// or which dialects the CLI runs. They belong in the generated status block,
// where a new driver package changes them; elsewhere they go stale.
var statusProse = regexp.MustCompile("(?i)(not usable|unusable|is usable|are usable|usable through|postgresql-only|only postgresql|only supported database|`momo migrate`[^.]{0,80}postgresql only)")

var spaces = regexp.MustCompile(`\s+`)

func problems(text string) []string {
	var out []string
	flat := spaces.ReplaceAllString(text, " ")
	for _, m := range statusProse.FindAllStringIndex(flat, -1) {
		lo, hi := max(0, m[0]-40), min(len(flat), m[1]+40)
		out = append(out, "..."+flat[lo:hi]+"...")
	}
	return out
}

// Catches: hand-written dialect-status prose creeping back next to the
// generated block, in the README, llms.txt or any page under docs/.
func TestNoStatusProseOutsideBlocks(t *testing.T) {
	root := repoRoot(t)
	files := []string{"README.md", "llms.txt"}
	pages, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		files = append(files, "docs/"+filepath.Base(p))
	}
	for _, f := range files {
		doc, err := readFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range problems(StripBlocks(doc)) {
			t.Errorf("%s: hand-written status prose outside a generated block: %s", f, p)
		}
	}
	for _, f := range []string{"README.md", "llms.txt", "docs/ai-guide.md", "docs/supported-versions.md", "docs/migrations.md", "docs/capabilities.md"} {
		doc, err := readFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(doc, "<!-- generated:status -->") {
			t.Errorf("%s has no generated status block", f)
		}
	}
}

// Catches: a feature package's documentation saying which database is usable
// (jsonop once said MySQL is not usable while the MySQL suites ran), a claim
// that goes stale the way the docs pages did.
func TestPackageDocsHoldNoStatusProse(t *testing.T) {
	root := repoRoot(t)
	list, err := readFile(filepath.Join(root, "internal", "repocheck", "testdata", "packages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, line := range strings.Split(list, "\n") {
		role, dir, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || role != "feature" {
			continue
		}
		paths, err := filepath.Glob(filepath.Join(root, dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range paths {
			if strings.HasSuffix(p, "_test.go") {
				continue
			}
			src, err := readFile(p)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.ParseComments|parser.PackageClauseOnly)
			if err != nil {
				t.Fatal(err)
			}
			if f.Doc == nil {
				continue
			}
			checked++
			for _, pr := range problems(f.Doc.Text()) {
				t.Errorf("%s/%s: package comment holds status prose: %s", dir, filepath.Base(p), pr)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no feature package comment was checked; packages.txt or the parser changed")
	}
}

func TestProseBanCatchesThePhrases(t *testing.T) {
	for _, s := range []string{
		"MySQL is not usable in this release",
		"SQLite is usable through the cgo driver",
		"the `momo migrate` command is PostgreSQL-only",
		"the `momo migrate` command runs PostgreSQL only",
		"PostgreSQL with pgvector is the only supported database for it",
		"MySQL is\nnot usable",
	} {
		if len(problems(s)) == 0 {
			t.Errorf("the prose ban does not catch %q", s)
		}
	}
	if len(problems("Array columns are PostgreSQL only; zero value must not be used")) != 0 {
		t.Error("the prose ban catches a per-feature PostgreSQL-only statement")
	}
}

// inferDriver reads what a driver package's source says about itself, to check
// its declaration against: the dialect identifiers it uses, build constraints
// for cgo and Windows, and its Capabilities literals.
func inferDriver(t *testing.T, root, name string) (dialects map[dialect.Name]bool, cgo, noWindows bool, vectors string) {
	t.Helper()
	dialects = map[dialect.Name]bool{}
	seen := map[string]bool{}
	cgoRE := regexp.MustCompile(`(^|[^!\w])cgo\b`)
	paths, err := filepath.Glob(filepath.Join(root, "driver", name, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		src, err := readFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(src, "\n") {
			if strings.HasPrefix(l, "//go:build ") {
				cgo = cgo || cgoRE.MatchString(l)
				noWindows = noWindows || strings.Contains(l, "!windows")
			}
		}
		f, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := n.X.(*ast.Ident); ok && id.Name == "dialect" {
					switch n.Sel.Name {
					case "Postgres":
						dialects[dialect.Postgres] = true
					case "SQLite":
						dialects[dialect.SQLite] = true
					case "MySQL", "MySQLFlavor":
						dialects[dialect.MySQL] = true
					}
				}
			case *ast.CompositeLit:
				if sel, ok := n.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Capabilities" {
					v := "no"
					for _, e := range n.Elts {
						kv, ok := e.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Vectors" {
							v = "server"
							if id, ok := kv.Value.(*ast.Ident); ok && id.Name == "true" {
								v = "yes"
							} else if ok && id.Name == "false" {
								v = "no"
							}
						}
					}
					seen[v] = true
				}
			}
			return true
		})
	}
	switch {
	case len(seen) == 0:
		vectors = "no"
	case len(seen) == 1:
		for v := range seen {
			vectors = v
		}
	default:
		vectors = "server"
	}
	return dialects, cgo, noWindows, vectors
}

// Catches: a driver package's declaration drifting from what its source does,
// for example a package that starts needing cgo while still declaring cgo=false.
func TestDriverDeclarationsMatchSource(t *testing.T) {
	root := repoRoot(t)
	drivers, err := scanDrivers(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(drivers) < 4 {
		t.Fatalf("found %d driver declarations, want at least 4", len(drivers))
	}
	for _, d := range drivers {
		dialects, cgo, noWindows, vectors := inferDriver(t, root, strings.TrimPrefix(d.Dir, "driver/"))
		if !dialects[d.Dialect] {
			t.Errorf("%s declares dialect %s but its source never refers to it", d.Dir, d.Dialect)
		}
		if d.NeedsCGO != cgo {
			t.Errorf("%s declares cgo=%v, its build constraints say %v", d.Dir, d.NeedsCGO, cgo)
		}
		if d.Windows == noWindows {
			t.Errorf("%s declares windows=%v, its build constraints say noWindows=%v", d.Dir, d.Windows, noWindows)
		}
		if d.Vectors != vectors {
			t.Errorf("%s declares vectors=%s, its Capabilities say %s", d.Dir, d.Vectors, vectors)
		}
	}
}

// Catches: a malformed declaration being read as something else.
func TestBadDeclarationsFail(t *testing.T) {
	for name, decl := range map[string]string{
		"missing key": "dialect=sqlite usable=true cgo=true windows=false",
		"bad bool":    "dialect=sqlite usable=maybe cgo=true windows=false vectors=yes",
		"bad dialect": "dialect=oracle usable=true cgo=true windows=false vectors=yes",
		"bad vectors": "dialect=sqlite usable=true cgo=true windows=false vectors=sometimes",
		"extra key":   "dialect=sqlite usable=true cgo=true windows=false vectors=yes color=red",
	} {
		root := fakeRepo(t, false)
		write(t, root, "driver/odd/status.go", "package odd\n\n//momo:driver "+decl+"\n")
		if _, err := scanDrivers(root); err == nil {
			t.Errorf("%s: scanDrivers accepted %q", name, decl)
		}
	}
	root := fakeRepo(t, false)
	write(t, root, "driver/odd/status.go", "package odd\n\n//momo:driver dialect=sqlite usable=true cgo=true windows=false vectors=yes\n//momo:driver dialect=mysql usable=true cgo=true windows=false vectors=yes\n")
	if _, err := scanDrivers(root); err == nil {
		t.Error("scanDrivers accepted two declarations in one package")
	}
}

// Catches: the CLI's dialect switch changing (a second dialect added, or the
// function renamed) without the docs following.
func TestCLIDialectsComeFromTheDialectSwitch(t *testing.T) {
	got, err := cliDialects(fakeRepo(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got["postgres"] {
		t.Errorf("fake CLI dialects = %v, want only postgres", got)
	}
	real, err := cliDialects(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if !real["postgres"] {
		t.Errorf("cmd/momo dialectImpl dialects = %v, want postgres in it", real)
	}
	empty := t.TempDir()
	write(t, empty, "cmd/momo/m.go", "package main\n")
	if _, err := cliDialects(empty); err == nil {
		t.Error("cliDialects accepted a cmd/momo without dialectImpl")
	}
}

// Catches: a new direct dependency shipping with an empty "Used by" cell.
func TestDependencyWithoutNoteFailsGeneration(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.26\n\nrequire example.com/new v1.0.0\n")
	if _, err := dependenciesBlock(root); err == nil {
		t.Error("dependenciesBlock accepted a direct dependency with no note")
	}
}
