// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/fromdb"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// shopSchema is what the inspector reports for a small database that uses
// most of what the generated models can declare.
func shopSchema() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Enums:         []schema.Enum{{Name: "user_role", Values: []string{"admin", "member"}}},
		Tables: []schema.Table{
			{
				Name: "orders",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "serial"},
					{Name: "user_id", Type: "bigint"},
					{Name: "note", Type: "text", Default: "'a,b'::text", Comment: "free text, shown to staff"},
					{Name: "placed_at", Type: "timestamp without time zone"},
					{Name: "total", Type: "integer"},
					{Name: "labels", Type: "text[]", Nullable: true},
					{Name: "deleted_at", Type: "timestamp with time zone", Nullable: true},
				},
				PrimaryKey: []string{"id"},
				Uniques:    []schema.Unique{{Name: "orders_user_total_key", Columns: []string{"user_id", "total"}}},
				Indexes: []schema.Index{
					{
						Name:  "orders_user_placed_idx",
						Parts: []schema.IndexPart{{Column: "user_id"}, {Column: "placed_at", Desc: true}},
					},
					{
						Name: "orders_live_key", Unique: true, Where: "(deleted_at IS NULL)",
						Parts: []schema.IndexPart{{Column: "note"}},
					},
					{Name: "orders_total_idx", Parts: []schema.IndexPart{{Column: "total"}}},
				},
				ForeignKeys: []schema.ForeignKey{{
					Name: "orders_user_id_fkey", Columns: []string{"user_id"}, RefTable: "users",
					RefColumns: []string{"id"}, OnDelete: "CASCADE",
				}},
				Checks: []schema.Check{{Name: "orders_total_check", Expr: "total > 0"}},
			},
			{
				Name: "users",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "email", Type: "character varying(200)"},
					{Name: "full_name", Type: "text", Nullable: true},
					{Name: "created_at", Type: "timestamp with time zone", Default: "now()"},
					{Name: "meta", Type: "jsonb", Nullable: true},
					{Name: "role", Type: "user_role", Enum: "user_role", Default: "'member'::user_role"},
					{Name: "balance", Type: "numeric(10,2)"},
					{Name: "score", Type: "double precision", Nullable: true},
				},
				PrimaryKey: []string{"id"},
				Indexes: []schema.Index{{
					Name: "users_email_key", Unique: true, Parts: []schema.IndexPart{{Column: "email"}},
				}},
			},
		},
	}
}

// TestGenerateFromDBRoundTrip writes the models for an inspected schema into
// a module, loads them with the real generator and checks that the schema
// they declare is the schema the init snapshot is written from.
func TestGenerateFromDBRoundTrip(t *testing.T) {
	res, err := fromdb.Generate(shopSchema(), fromdb.Options{Package: "models"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	src := t.TempDir()
	for _, f := range res.Files {
		writeFile(t, filepath.Join(src, "models", f.Name), string(f.Content))
	}
	root := copyModule(t, src)
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\npackages:\n  - ./models\ndialects: [postgres]\n")
	chdir(t, root)

	var out, errb bytes.Buffer
	if code := Run([]string{"generate"}, &out, &errb); code != 0 {
		t.Fatalf("generate: exit %d\n%s%s", code, out.String(), errb.String())
	}
	g, diags, err := gen.Load([]string{"./models"}, gen.Config{
		Dir: root, Naming: naming.Momo{}, Dialects: []dialect.Name{dialect.Postgres},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) > 0 {
		t.Fatalf("the generated models have diagnostics: %v", diags)
	}
	changes, _, err := migrate.Diff(res.Snapshot, g.Schema, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) > 0 {
		var lines []string
		for _, c := range changes {
			lines = append(lines, string(c.Kind)+" "+c.Table+" "+c.Name)
		}
		t.Fatalf("the models drift from the snapshot:\n%s", strings.Join(lines, "\n"))
	}
}

func TestGenerateFromDBFlagErrors(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	var out, errb bytes.Buffer
	if code := Run([]string{"generate", "--out", "x"}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "--from-db") {
		t.Fatalf("--out alone: exit %d, %q", code, errb.String())
	}
	errb.Reset()
	if code := Run([]string{"generate", "--from-db", "NO_SUCH_VARIABLE_FOR_MOMO", "./..."}, &out, &errb); code != exitUsage {
		t.Fatalf("patterns with --from-db: exit %d, %q", code, errb.String())
	}
	errb.Reset()
	t.Setenv("MOMO_EMPTY_URL_TEST", "")
	if code := Run([]string{"generate", "--from-db", "MOMO_EMPTY_URL_TEST"}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "MOMO_EMPTY_URL_TEST") {
		t.Fatalf("empty variable: exit %d, %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "models")); err == nil {
		t.Fatal("a refused run created the model directory")
	}
}

// TestGenerateFromDBRefusesOtherDialects: the inspector reads PostgreSQL only,
// so a project configured for another dialect is refused with exit 2 before a
// connection is opened or a file is written.
func TestGenerateFromDBRefusesOtherDialects(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\npackages:\n  - ./models\ndialects: [mysql]\n")
	t.Setenv("MOMO_DIALECT_URL_TEST", "mysql://u:p@127.0.0.1:1/db")
	var out, errb bytes.Buffer
	code := Run([]string{"generate", "--from-db", "MOMO_DIALECT_URL_TEST"}, &out, &errb)
	if code != exitUsage || !strings.Contains(errb.String(), "PostgreSQL databases only, not mysql") {
		t.Fatalf("exit %d, %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, "models")); err == nil {
		t.Fatal("a refused run created the model directory")
	}
}

func TestIntrospectConfigFromYAML(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	writeFile(t, filepath.Join(root, "momo.yaml"), `version: 1
introspect:
  package: shop
  skip: ["_prisma_*"]
  types:
    inet: {go: string}
  columns:
    - match: "*.updated_at"
      tags: [updated_at]
  tables:
    users: {name: Account}
`)
	file, _, err := loadConfigFile("", root)
	if err != nil {
		t.Fatal(err)
	}
	in := file.Introspect
	if in.Package != "shop" || len(in.Skip) != 1 || in.Types["inet"].Go != "string" || in.Columns[0].Match != "*.updated_at" || in.Tables["users"].Name != "Account" {
		t.Fatalf("introspect = %+v", in)
	}
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\nintrospect:\n  columns:\n    - match: nodot\n")
	if _, _, err := loadConfigFile("", root); err == nil || !strings.Contains(err.Error(), "introspect.columns[0].match") {
		t.Fatalf("bad rule: err = %v", err)
	}
}

// fromDBTree writes the generated models of shopSchema into a new directory
// and returns it with the regenerated files. rewrite turns the content of the
// file under test into what is on disk.
func fromDBTree(t *testing.T, rewrite func(content string) string) (dir string, res *fromdb.Result) {
	t.Helper()
	res, err := fromdb.Generate(shopSchema(), fromdb.Options{Package: "models"})
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	for _, f := range res.Files {
		content := string(f.Content)
		if f.Name == "users_model.go" {
			content = rewrite(content)
		}
		writeFile(t, filepath.Join(dir, f.Name), content)
	}
	return dir, res
}

// staleHeader gives content a header hash that is not the hash of its body.
func staleHeader(content string) string {
	i := strings.Index(content, "\n")
	return "// momo:fromdb sha256=" + strings.Repeat("0", 64) + content[i:]
}

// toCRLF is a checkout with core.autocrlf=true.
func toCRLF(content string) string { return strings.ReplaceAll(content, "\n", "\r\n") }

// runFromDBPlan is the file part of generate --from-db: plan, report, and for
// a plain run apply.
func runFromDBPlan(t *testing.T, dir string, res *fromdb.Result, check bool) (stale bool, out string) {
	t.Helper()
	var buf bytes.Buffer
	l := &cli{out: &buf, errOut: &buf, cwd: dir}
	actions, err := fromdb.Plan(dir, res.Files, false)
	if err != nil {
		t.Fatal(err)
	}
	stale = l.reportActions(dir, actions, check)
	if !check {
		if _, err := fromdb.Apply(dir, actions, writeModelFile); err != nil {
			t.Fatal(err)
		}
	}
	return stale, buf.String()
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestFromDBBodyEqualStaleHeader: a file whose body equals the regenerated
// body but whose header hash is stale is current for --check (issue 128), and
// a plain run rewrites only its first line, with no conflict and no .new file.
// With CRLF line endings the same holds and the CRLF endings are kept.
//
// Catches: --check reporting "stale", or a plain run reporting a conflict and
// writing a .new file, for a model whose body is already the regenerated one.
func TestFromDBBodyEqualStaleHeader(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  func(string) string
	}{
		{"lf", func(s string) string { return s }},
		{"crlf", toCRLF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want string
			dir, res := fromDBTree(t, func(c string) string {
				want = tc.eol(c)
				return tc.eol(staleHeader(c))
			})
			path := filepath.Join(dir, "users_model.go")
			before := readTestFile(t, path)

			if stale, out := runFromDBPlan(t, dir, res, true); stale || strings.Contains(out, "stale") {
				t.Fatalf("--check must report the file as current, got:\n%s", out)
			}
			if readTestFile(t, path) != before {
				t.Fatal("--check wrote the file")
			}

			if _, out := runFromDBPlan(t, dir, res, false); strings.Contains(out, "conflict") {
				t.Fatalf("a plain run must not report a conflict:\n%s", out)
			}
			if _, err := os.Stat(path + ".new"); err == nil {
				t.Fatal("a plain run wrote a .new file")
			}
			if got := readTestFile(t, path); got != want {
				t.Fatalf("only the first line may change.\ngot:\n%q\nwant:\n%q", truncate(got, 120), truncate(want, 120))
			}
			if stale, out := runFromDBPlan(t, dir, res, true); stale {
				t.Fatalf("--check after the rewrite: %s", out)
			}
		})
	}
}

// TestFromDBHandEditStaysConflict: a body that differs from the regenerated
// one is still stale for --check and a conflict with a .new file for a plain
// run, in LF and in CRLF, and the file is not touched.
//
// Catches: the body comparison of issue 128 accepting a real hand edit, or
// CRLF endings making the comparison differ for the wrong reason.
func TestFromDBHandEditStaysConflict(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  func(string) string
	}{
		{"lf", func(s string) string { return s }},
		{"crlf", toCRLF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, res := fromDBTree(t, func(c string) string {
				return tc.eol(staleHeader(c) + "\n// hand written\n")
			})
			path := filepath.Join(dir, "users_model.go")
			before := readTestFile(t, path)

			if stale, out := runFromDBPlan(t, dir, res, true); !stale || !strings.Contains(out, "stale: users_model.go") {
				t.Fatalf("--check must report the edited file as stale, got:\n%s", out)
			}
			if _, out := runFromDBPlan(t, dir, res, false); !strings.Contains(out, "conflict: users_model.go") {
				t.Fatalf("a plain run must report a conflict:\n%s", out)
			}
			if readTestFile(t, path) != before {
				t.Fatal("a conflicting file was changed")
			}
			if _, err := os.Stat(path + ".new"); err != nil {
				t.Fatalf("the regenerated version is missing: %v", err)
			}
		})
	}
}

// truncate shortens s to at most n bytes for a failure message.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// planKind plans the users model of dir and returns the kind of its action.
func planKind(t *testing.T, dir string, res *fromdb.Result) string {
	t.Helper()
	actions, err := fromdb.Plan(dir, res.Files, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions {
		if a.File == "users_model.go" {
			return a.Kind
		}
	}
	t.Fatal("no action for users_model.go")
	return ""
}

// TestFromDBCRLFCurrentFileIsUnchanged: a CRLF copy of a file whose header and
// body are both current is unchanged.
//
// Catches: a core.autocrlf checkout of an up-to-date model being reported as a
// conflict or stale.
func TestFromDBCRLFCurrentFileIsUnchanged(t *testing.T) {
	dir, res := fromDBTree(t, toCRLF)
	if got := planKind(t, dir, res); got != fromdb.ActionUnchanged {
		t.Fatalf("kind = %q, want %q", got, fromdb.ActionUnchanged)
	}
	if stale, out := runFromDBPlan(t, dir, res, true); stale {
		t.Fatalf("--check: %s", out)
	}
}

// TestFromDBEditedDatabaseUnchanged: a hand-edited file whose header hash is
// still the hash of the regenerated body stays quiet, in LF and in CRLF.
//
// Catches: the "edited, database did not change" rule comparing the header of a
// CRLF file without normalising its line endings, so that CRLF conflicts where
// LF is unchanged.
func TestFromDBEditedDatabaseUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  func(string) string
	}{
		{"lf", func(s string) string { return s }},
		{"crlf", toCRLF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, res := fromDBTree(t, func(c string) string { return tc.eol(c + "\n// hand written\n") })
			if got := planKind(t, dir, res); got != fromdb.ActionUnchanged {
				t.Fatalf("kind = %q, want %q", got, fromdb.ActionUnchanged)
			}
			if stale, out := runFromDBPlan(t, dir, res, true); stale {
				t.Fatalf("--check: %s", out)
			}
		})
	}
}

// TestFromDBLineBelowHeaderIsAnEdit: a line added between the header comment
// and the blank line that starts the body, with a stale hash, is a hand edit
// and not a body-equal file, in LF and in CRLF.
//
// Catches: the body comparison ignoring text between the header and the first
// blank line, such as a build constraint, and calling the file current.
func TestFromDBLineBelowHeaderIsAnEdit(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  func(string) string
	}{
		{"lf", func(s string) string { return s }},
		{"crlf", toCRLF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, res := fromDBTree(t, func(c string) string {
				lines := strings.SplitN(staleHeader(c), "\n", 3)
				return tc.eol(lines[0] + "\n" + lines[1] + "\n//go:build integration\n" + lines[2])
			})
			if stale, out := runFromDBPlan(t, dir, res, true); !stale || !strings.Contains(out, "stale: users_model.go") {
				t.Fatalf("--check must report the file as stale, got:\n%s", out)
			}
			if _, out := runFromDBPlan(t, dir, res, false); !strings.Contains(out, "conflict: users_model.go") {
				t.Fatalf("a plain run must report a conflict:\n%s", out)
			}
		})
	}
}
