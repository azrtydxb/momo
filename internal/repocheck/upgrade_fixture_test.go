// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// The upgrade fixture is a program written against the root package before the
// split (testdata/upgrade_fixture/old). These tests prove three things about
// docs/upgrading.md:
//
//   - the fixture uses every pair of testdata/moves/*.txt, so a move added
//     later cannot go unrehearsed;
//   - the fixture does not compile against the current module, and the compiler
//     reports each moved name as undefined;
//   - the documented rewrite recipe, run on a copy of the fixture, produces a
//     program that compiles against the new packages.
//
// The recipe shells out to sh, awk and gofmt, so the build tests skip on
// Windows; the coverage test runs everywhere.

const fixtureDir = "internal/repocheck/testdata/upgrade_fixture"

// methodParts splits the old name of a pair, written after "momo.", into its
// parts: one for a function, type or constant, two for a method.
func methodParts(p movePair) []string {
	return strings.Split(strings.TrimPrefix(p.old, "momo."), ".")
}

// fixtureUses reports whether the fixture source uses the old name of p. A
// plain identifier is looked for as momo.<name>; a method of Tx or DB through
// the conventional receiver variable (tx, db) so the two forms stay apart; any
// other method by its name alone.
func fixtureUses(src string, p movePair) bool {
	parts := methodParts(p)
	var re string
	switch {
	case len(parts) == 1:
		re = `\bmomo\.` + parts[0] + `\b`
	case parts[0] == "Tx":
		re = `\btx\.` + parts[1] + `\(`
	case parts[0] == "DB":
		re = `\bdb\.` + parts[1] + `\(`
	default:
		re = `\.` + parts[1] + `\(`
	}
	return regexp.MustCompile(re).MatchString(src)
}

func TestUpgradeFixtureCoversMoves(t *testing.T) {
	src := readRepoFile(t, filepath.FromSlash(fixtureDir), "old", "main.go")
	moves := loadMoves(t)
	if len(moves) == 0 {
		t.Fatal("no pairs in testdata/moves")
	}
	for _, p := range moves {
		if !fixtureUses(src, p) {
			t.Errorf("%s/old/main.go never uses %s (new: %s); add a use to the fixture", fixtureDir, p.old, p.new)
		}
	}
}

func skipWithoutShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the rewrite recipe needs sh, awk and gofmt")
	}
}

// goBuild builds the package at dir (relative to the repository root) to
// nowhere and returns the compiler output and error.
func goBuild(t *testing.T, dir string, flags ...string) (string, error) {
	t.Helper()
	args := append([]string{"build"}, flags...)
	args = append(args, "-o", os.DevNull, "./"+dir)
	cmd := exec.Command("go", args...) // #nosec G204 -- fixed arguments plus a path inside the repository
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestUpgradeFixtureOldFailsToCompile(t *testing.T) {
	skipWithoutShell(t)
	out, err := goBuild(t, fixtureDir+"/old", "-gcflags=-e")
	if err == nil {
		t.Fatal("the old fixture compiled against the current module; every moved name must be gone from momo")
	}
	for _, p := range loadMoves(t) {
		parts := methodParts(p)
		var want string
		switch {
		case len(parts) == 1:
			want = "undefined: " + p.old
		case parts[0] == "Tx" || parts[0] == "DB":
			want = "." + parts[1] + " undefined"
		default:
			continue // a method of a moved type: the type itself is reported
		}
		if !strings.Contains(out, want) {
			t.Errorf("compiler output lacks %q for %s:\n%s", want, p.old, out)
		}
	}
}

// recipeScript returns the first sh block under the "Rewrite recipe" heading
// of the page.
func recipeScript(t *testing.T, page string) string {
	t.Helper()
	in, fenced := false, false
	var lines []string
	for _, line := range strings.Split(page, "\n") {
		switch {
		case strings.HasPrefix(line, "#"):
			if fenced {
				break
			}
			in = strings.TrimSpace(line) == "### Rewrite recipe"
		case in && !fenced && strings.TrimSpace(line) == "```sh":
			fenced = true
		case in && fenced && strings.TrimSpace(line) == "```":
			return strings.Join(lines, "\n")
		case in && fenced:
			lines = append(lines, line)
		}
	}
	t.Fatal("docs/upgrading.md has no ```sh block under \"### Rewrite recipe\"")
	return ""
}

// rewriteMethods applies the by-hand rule of the recipe for methods that
// became functions: recv.Old(ctx, args) becomes pkg.New(ctx, recv, args). The
// advisory functions take a transaction, so a DB-form call passes the
// transaction tx the caller opened.
func rewriteMethods(src string, moves []movePair) string {
	for _, p := range moves {
		parts := methodParts(p)
		if len(parts) != 2 || (parts[0] != "Tx" && parts[0] != "DB") {
			continue
		}
		recv := strings.ToLower(parts[0])
		arg := recv
		if strings.HasPrefix(p.new, "advisory.") {
			arg = "tx"
		}
		re := regexp.MustCompile(`\b` + recv + `\.` + parts[1] + `\(ctx, `)
		src = re.ReplaceAllString(src, p.new+"(ctx, "+arg+", ")
	}
	return src
}

// addImports adds the import of every new package the source now uses, which
// is what goimports does for a real code base.
func addImports(src string, moves []movePair) string {
	seen := map[string]bool{}
	var imports []string
	for _, p := range moves {
		pkg, _, _ := strings.Cut(p.new, ".")
		if seen[pkg] || !regexp.MustCompile(`\b`+pkg+`\.`).MatchString(src) {
			continue
		}
		seen[pkg] = true
		imports = append(imports, "\t\""+p.path+"\"\n")
	}
	return strings.Replace(src, "import (\n", "import (\n"+strings.Join(imports, ""), 1)
}

func TestUpgradeFixtureRecipeCompiles(t *testing.T) {
	skipWithoutShell(t)
	root := repoRoot(t)
	page := readRepoFile(t, "docs", "upgrading.md")
	script := recipeScript(t, page)
	moves := loadMoves(t)

	parent := filepath.Join(root, filepath.FromSlash(fixtureDir))
	work, err := os.MkdirTemp(parent, "new-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(work) })

	old := readRepoFile(t, filepath.FromSlash(fixtureDir), "old", "main.go")
	if err := os.WriteFile(filepath.Join(work, "upgrading.md"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	mainGo := filepath.Join(work, "main.go")
	if err := os.WriteFile(mainGo, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "-c", script) // #nosec G204 -- the recipe block of the repository's own docs page
	cmd.Dir = work
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("the documented recipe failed: %v\n%s", err, stderr.String())
	}

	rewritten, err := os.ReadFile(mainGo) // #nosec G304 -- a file this test just wrote
	if err != nil {
		t.Fatal(err)
	}
	src := addImports(rewriteMethods(string(rewritten), moves), moves)
	if err := os.WriteFile(mainGo, []byte(src), 0o600); err != nil { // #nosec G703 -- a file in this test's own work directory
		t.Fatal(err)
	}
	for _, p := range moves {
		if parts := methodParts(p); len(parts) == 1 && regexp.MustCompile(`\bmomo\.`+parts[0]+`\b`).MatchString(src) {
			t.Errorf("the recipe left %s in the program", p.old)
		}
	}

	rel, err := filepath.Rel(root, work)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := goBuild(t, filepath.ToSlash(rel), "-gcflags=-e"); err != nil {
		t.Fatalf("the rewritten fixture does not compile: %v\n%s\n--- source ---\n%s", err, out, src)
	}
}
