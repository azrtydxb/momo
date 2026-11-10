// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// What counts as an identifier reference in the docs. Only an inline code span
// (backticks on one line, outside a fenced block) whose whole content has one
// of these shapes is checked, after type arguments such as [T] and a trailing
// call such as (ctx) are removed:
//
//	pkg.Name           pkg is the name of a package of this module (momo, types, vecop, ...)
//	pkg.Type.Member    a method or field of a type of that package
//	Type.Member        Type is an exported type of the root package, momo
//
// pkg is checked only for the packages in APIPackages (momo, dialect, schema,
// types, vector, migrate, momolog, momotest and the feature packages). Other module packages share names
// with third-party libraries (importer/gorm and gorm.Model, migrate/mysql and
// mysql.MySQLError), so a reference to them cannot be told apart. docs/upgrading.md
// is checked except for the old-identifier column of its table.
//
// Name and Member must start with an upper-case letter. Everything else is
// prose and is not checked: SQL, file names (go.mod), other libraries
// (sql.DB, pgx.Conn: their first segment is not a module package) and
// lower-case members. A reference that has the shape but does not resolve
// fails; there is no allow list, so fix the doc (or write the name as plain
// text). A bare name such as `Select` is not resolved here; it means the root
// package, which TestBareNamesAreRootNames checks.

var (
	spanRe = regexp.MustCompile("`([^`\n]+)`")
	refRe  = regexp.MustCompile(`^\*?([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)(?:\.([A-Za-z_][A-Za-z0-9_]*))?$`)
)

type docRef struct {
	line int
	text string // the span without backticks, after trimming a call or type-argument list
}

// stripTypeArgs removes every [...] group (type arguments or an index) and
// cuts the span at the first '(' (a call), so Relation[T].Has and
// momo.Select[T](db) check as Relation.Has and momo.Select.
func stripTypeArgs(s string) string {
	var sb strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '[':
			depth++
		case r == ']':
			if depth > 0 {
				depth--
			}
		case depth > 0:
		case r == '(':
			return sb.String()
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

var fenceRe = regexp.MustCompile("^\\s*(`{3,}|~{3,})")

// extractRefs returns the identifier-shaped inline spans of one page, with
// their 1-based line numbers. Fenced code blocks (``` or ~~~, closed by a fence
// of the same character and at least the same length) are skipped. With
// skipFirstCell, spans in the first cell of a table row are skipped too: the
// upgrade table's first column names identifiers that no longer exist.
func extractRefs(text string, skipFirstCell bool) []docRef {
	var out []docRef
	for _, r := range extractSpans(text, skipFirstCell) {
		if refRe.MatchString(r.text) {
			out = append(out, r)
		}
	}
	return out
}

// extractSpans returns every inline code span of one page, after stripTypeArgs,
// under the same fence and first-cell rules as extractRefs.
func extractSpans(text string, skipFirstCell bool) []docRef {
	var out []docRef
	var open string // the opening fence; empty outside a block
	for i, line := range strings.Split(text, "\n") {
		if m := fenceRe.FindStringSubmatch(line); m != nil {
			f := m[1]
			rest := strings.TrimSpace(line[strings.Index(line, f)+len(f):])
			switch {
			case open == "":
				open = f
				continue
			case f[0] == open[0] && len(f) >= len(open) && strings.Trim(rest, string(f[0])) == "":
				open = ""
				continue
			}
		}
		if open != "" {
			continue
		}
		limit := 0
		if t := strings.TrimSpace(line); skipFirstCell && strings.HasPrefix(t, "|") {
			if k := strings.Index(t[1:], "|"); k >= 0 {
				limit = strings.Index(line, t) + 1 + k
			}
		}
		for _, loc := range spanRe.FindAllStringSubmatchIndex(line, -1) {
			if loc[0] < limit {
				continue
			}
			out = append(out, docRef{line: i + 1, text: stripTypeArgs(line[loc[2]:loc[3]])})
		}
	}
	return out
}

func upperFirst(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

// resolveRef reports whether ref names a real identifier. checked is false when
// the reference is not about this module (see the rules above).
func resolveRef(pkgs []*Package, ref string) (ok, checked bool) {
	m := refRe.FindStringSubmatch(ref)
	if m == nil {
		return true, false
	}
	a, b, c := m[1], m[2], m[3]
	var named []*Package
	var root *Package
	for _, p := range pkgs {
		if p.Name == a {
			named = append(named, p)
		}
		if p.Dir == "." {
			root = p
		}
	}
	if len(named) > 0 {
		if !upperFirst(b) || (c != "" && !upperFirst(c)) {
			return true, false
		}
		for _, p := range named {
			if c == "" && p.HasTop(b) {
				return true, true
			}
			if c != "" && p.HasTop(b) && p.HasMember(b, c) {
				return true, true
			}
		}
		return false, true
	}
	if c == "" && root != nil && root.HasTop(a) && upperFirst(b) {
		return root.HasMember(a, b), true
	}
	return true, false
}

var bareRe = regexp.MustCompile(`^\*?([A-Z][A-Za-z0-9_]*)(?:\.([A-Z][A-Za-z0-9_]*))?$`)

// bareOwners returns the names of the feature packages that a bare span would
// have to be qualified with. A bare name in the docs means the root package,
// so a non-empty result is a name a reader would write as momo.Name and fail
// to compile. text is "Name" (a top-level identifier) or "Type.Member" (a
// method or field), with an optional leading *. The result is nil when the
// root package exports it, and when no checked package does (prose, other
// libraries).
func bareOwners(pkgs []*Package, text string) []string {
	m := bareRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	name, member := m[1], m[2]
	var owners []string
	for _, p := range pkgs {
		if p.Dir == "." {
			if p.HasTop(name) {
				return nil
			}
			continue
		}
		if p.HasTop(name) && (member == "" || p.HasMember(name, member)) {
			owners = append(owners, p.Name)
		}
	}
	sort.Strings(owners)
	return owners
}

const bareAllowFile = "testdata/bare_allow.txt"

// bareAllow reads bareAllowFile: one "path package reason" per line, meaning
// that the bare names of that package are intended on that page (its section
// names the package, or the name is a method or field there). The reason is
// required.
func bareAllow(t *testing.T) map[[2]string]bool {
	t.Helper()
	out := map[[2]string]bool{}
	for l := range readList(t, bareAllowFile) {
		f := strings.Fields(l)
		if len(f) < 3 {
			t.Errorf("%s: %q needs \"path package reason\"", bareAllowFile, l)
			continue
		}
		out[[2]string{f[0], f[1]}] = true
	}
	return out
}

// TestBareNamesAreRootNames: a bare backticked name such as `Case` means the
// root package. When only a feature package exports it, the doc must write
// pkg.Name (sqlfn.Case), or the page's package must be listed in
// bareAllowFile with a reason. Listed entries that match nothing are stale.
func TestBareNamesAreRootNames(t *testing.T) {
	root := repoRoot(t)
	pkgs, err := ReferencePackages(root)
	if err != nil {
		t.Fatal(err)
	}
	allow := bareAllow(t)
	used := map[[2]string]bool{}
	for _, f := range docPages(t, root) {
		rel, _ := filepath.Rel(root, f)
		rel = filepath.ToSlash(rel)
		if rel == "docs/reference.md" { // generated from the code
			continue
		}
		b, err := os.ReadFile(f) // #nosec G304 -- the repository's own docs
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range extractSpans(string(b), rel == "docs/upgrading.md") {
			owners := bareOwners(pkgs, r.text)
			if len(owners) == 0 {
				continue
			}
			ok := false
			for _, o := range owners {
				if allow[[2]string{rel, o}] {
					used[[2]string{rel, o}] = true
					ok = true
				}
			}
			if !ok {
				var sug []string
				for _, o := range owners {
					sug = append(sug, o+"."+strings.TrimPrefix(r.text, "*"))
				}
				t.Errorf("%s:%d: bare `%s` means the root package, which does not export it; write `%s` (qualify, do not drop the backticks), or list the page and package in %s with a reason", rel, r.line, r.text, strings.Join(sug, "` or `"), bareAllowFile)
			}
		}
	}
	for k := range allow {
		if !used[k] {
			t.Errorf("%s: %s %s matches no bare name; remove the line", bareAllowFile, k[0], k[1])
		}
	}
}

func TestBareOwners(t *testing.T) {
	root := parseSrc(t, ".", "package momo\nfunc Select() {}\nfunc Func() {}\ntype Relation struct{}\nfunc (Relation) Has() {}\n")
	sqlfn := parseSrc(t, "sqlfn", "package sqlfn\nfunc Case() {}\nfunc Func() {}\n")
	migrate := parseSrc(t, "migrate", "package migrate\ntype Engine struct{}\nfunc (Engine) Apply() {}\n")
	pkgs := []*Package{root, sqlfn, migrate}
	for name, want := range map[string]string{
		"Case":         "sqlfn",
		"*Case":        "sqlfn",
		"Select":       "",
		"Func":         "",
		"Prose":        "",
		"Relation.Has": "",
		"Engine.Apply": "migrate",
		"Engine.Nope":  "", // no checked package has that member
		"Other.Apply":  "",
	} {
		if got := strings.Join(bareOwners(pkgs, name), ","); got != want {
			t.Errorf("bareOwners(%q) = %q, want %q", name, got, want)
		}
	}
	// A leading * and a [] prefix do not hide a name.
	var got []string
	for _, r := range extractSpans("`*Case` `[]Case` `Engine[T].Apply` `momo.Select`", false) {
		got = append(got, r.text)
	}
	if want := "*Case,Case,Engine.Apply,momo.Select"; strings.Join(got, ",") != want {
		t.Errorf("extractSpans = %s, want %s", strings.Join(got, ","), want)
	}
}

// docPages returns docs/*.md (and one level below), README.md and llms.txt.
func docPages(t *testing.T, root string) []string {
	t.Helper()
	files := docFiles(t, root)
	files = append(files, filepath.Join(root, "README.md"), filepath.Join(root, "llms.txt"))
	sort.Strings(files)
	return files
}

func TestDocsIdentifiersResolve(t *testing.T) {
	root := repoRoot(t)
	pkgs, err := ReferencePackages(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range docPages(t, root) {
		b, err := os.ReadFile(f) // #nosec G304 -- the repository's own docs
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, f)
		// docs/upgrading.md names the identifiers that were removed in the first
		// column of its table; the new identifiers and the prose are checked.
		upgrading := filepath.ToSlash(rel) == "docs/upgrading.md"
		for _, r := range extractRefs(string(b), upgrading) {
			ok, was := resolveRef(pkgs, r.text)
			if !was {
				continue
			}
			checked++
			if !ok {
				t.Errorf("%s:%d: `%s` does not name an identifier of this module; fix the name or write it as plain text", filepath.ToSlash(rel), r.line, r.text)
			}
		}
	}
	if checked < 50 {
		t.Fatalf("only %d identifier references were checked; the extraction is broken", checked)
	}
}

func parseSrc(t *testing.T, dir, src string) *Package {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return ScanFiles(dir, []*ast.File{f})
}

func TestResolveRefRules(t *testing.T) {
	root := parseSrc(t, ".", `package momo
type Relation[P, C any] struct{ Name string }
func (r Relation[P, C]) Has() {}
type Hook func()
type Base struct{}
func (Base) Close() {}
type DB struct{ Base }
func Select[T any]() {}
type Expression[T any] interface{ And() }
type Predicate = Expression[bool]
type Codec interface {
	sql.Scanner
	SQLType() string
}
`)
	vecop := parseSrc(t, "vecop", "package vecop\nfunc Cosine() {}\n")
	pkgs := []*Package{root, vecop}
	for _, tc := range []struct {
		ref        string
		ok, checks bool
	}{
		{"momo.Hook", true, true},
		{"momo.NoSuchThing", false, true},
		{"vecop.Cosine", true, true},
		{"vecop.Missing", false, true},
		{"momo.Relation.Has", true, true},
		{"momo.Relation.Hsa", false, true},
		{"Relation.Has", true, true},
		{"Relation.Name", true, true},
		{"Relation.Nope", false, true},
		{"DB.Close", true, true}, // promoted through an embedded type
		{"momo.Select", true, true},
		{"Predicate.And", true, true},   // through the alias target
		{"Predicate.Nope", false, true}, // the alias target is checked
		{"momo.Codec.Scan", true, true}, // member of a cross-package embed
		{"momo.Codec.SQLType", true, true},
		{"sql.DB", true, false},    // another library
		{"go.mod", true, false},    // a file name
		{"momo.yaml", true, false}, // lower-case member
		{"Unknown.Method", true, false},
		{"a.B.C.D", true, false},
	} {
		ok, checked := resolveRef(pkgs, tc.ref)
		if checked != tc.checks || (checked && ok != tc.ok) {
			t.Errorf("resolveRef(%q) = ok %v checked %v, want ok %v checked %v", tc.ref, ok, checked, tc.ok, tc.checks)
		}
	}
}

func TestExtractRefs(t *testing.T) {
	text := strings.Join([]string{
		"see `momo.Hook` and `momo.Select[T](db)` and `SELECT x`",
		"```go",
		"`momo.Fenced`",
		"```",
		"`x` and `types.JSON` and `Relation[T].Has`",
		"~~~",
		"`momo.Tilde`",
		"~~~",
		"````md",
		"```",
		"`momo.Nested`",
		"```",
		"````",
		"`momo.After`",
		"| `momo.Old` | `momo.New` |",
	}, "\n")
	for _, tc := range []struct {
		skipFirst bool
		want      string
	}{
		{false, "1:momo.Hook,1:momo.Select,5:types.JSON,5:Relation.Has,14:momo.After,15:momo.Old,15:momo.New"},
		{true, "1:momo.Hook,1:momo.Select,5:types.JSON,5:Relation.Has,14:momo.After,15:momo.New"},
	} {
		var s []string
		for _, r := range extractRefs(text, tc.skipFirst) {
			s = append(s, fmt.Sprintf("%d:%s", r.line, r.text))
		}
		if got := strings.Join(s, ","); got != tc.want {
			t.Errorf("extractRefs(skipFirstCell=%v) = %s, want %s", tc.skipFirst, got, tc.want)
		}
	}
}

const undocumentedFile = "testdata/undocumented.txt"

// readList reads one entry per line, ignoring blank lines and # comments.
func readList(t *testing.T, path string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- fixed test data file
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			out[l] = true
		}
	}
	return out
}

// TestExportedIdentifiersDocumented is a ratchet. Every exported identifier of
// the core, leaf and feature packages needs a doc comment. The names that
// still lack one are listed in testdata/undocumented.txt, which may only
// shrink: a new undocumented name fails, and so does a listed name that is now
// documented or gone (remove its line).
func TestExportedIdentifiersDocumented(t *testing.T) {
	pkgs, err := ReferencePackages(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	listed := readList(t, undocumentedFile)
	current := map[string]bool{}
	total := 0
	for _, p := range pkgs {
		for _, id := range p.Idents {
			total++
			if strings.TrimSpace(id.Doc) == "" {
				current[id.Key(p.Name)] = true
			}
		}
	}
	if total < 200 {
		t.Fatalf("only %d exported identifiers found; the scan is broken", total)
	}
	var added, stale []string
	for k := range current {
		if !listed[k] {
			added = append(added, k)
		}
	}
	for k := range listed {
		if !current[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(added)
	sort.Strings(stale)
	if len(added) > 0 {
		t.Errorf("%d exported identifiers have no doc comment; write one (do not add them to %s):\n%s", len(added), undocumentedFile, strings.Join(added, "\n"))
	}
	if len(stale) > 0 {
		t.Errorf("%d names in %s are documented now or gone; remove these lines:\n%s", len(stale), undocumentedFile, strings.Join(stale, "\n"))
	}
}

// TestReferenceListsEveryIdentifier checks that the generated docs/reference.md
// names every exported identifier. After changing an exported name, run
// go generate ./internal/gendocs/... to refresh it.
func TestReferenceListsEveryIdentifier(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "docs", "reference.md")) // #nosec G304 -- the repository's own file
	if err != nil {
		t.Fatalf("docs/reference.md: %v (run go generate ./internal/gendocs/...)", err)
	}
	pkgs, err := ReferencePackages(root)
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, p := range pkgs {
		for _, id := range p.Idents {
			if !strings.Contains(string(b), "| `"+id.Key(p.Name)+"` ") {
				missing = append(missing, id.Key(p.Name))
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("docs/reference.md lacks %d identifiers; run go generate ./internal/gendocs/...:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}
