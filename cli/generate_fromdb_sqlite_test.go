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

// sqliteShopSchema is what the SQLite inspector reports for a small database
// that uses every feature --from-db models on SQLite.
func sqliteShopSchema() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.SQLite,
		Enums:         []schema.Enum{{Name: "accounts_status", Values: []string{"new", "active", "closed"}}},
		Tables: []schema.Table{
			{
				Name: "accounts",
				Columns: []schema.Column{
					{Name: "id", Type: "INTEGER", Identity: "by_default"},
					{Name: "tenant_id", Type: "INTEGER"},
					{Name: "email", Type: "VARCHAR(200)"},
					{Name: "nickname", Type: "TEXT", Nullable: true},
					{Name: "active", Type: "BOOLEAN", Default: "1"},
					{Name: "score", Type: "REAL", Default: "0"},
					{Name: "created_at", Type: "DATETIME", Default: "CURRENT_TIMESTAMP"},
					{Name: "status", Type: "TEXT", Default: "'new'", Enum: "accounts_status"},
					{Name: "email_lower", Type: "TEXT", Nullable: true, Generated: "lower(email)"},
					{Name: "initial", Type: "TEXT", Nullable: true, Generated: "substr(email, 1, 1)", GeneratedVirtual: true},
					{Name: "avatar", Type: "BLOB", Nullable: true},
				},
				PrimaryKey: []string{"id"},
				Checks:     []schema.Check{{Name: "accounts_score_check", Expr: "score >= 0"}},
				ForeignKeys: []schema.ForeignKey{{
					Name: "accounts_tenant_id_fkey", Columns: []string{"tenant_id"}, RefTable: "tenants",
					RefColumns: []string{"id"}, OnDelete: "CASCADE",
				}},
				Indexes: []schema.Index{
					{Name: "accounts_email_key", Unique: true, Parts: []schema.IndexPart{{Column: "tenant_id"}, {Column: "email"}}},
					{Name: "accounts_nickname_idx", Parts: []schema.IndexPart{{Column: "nickname", Desc: true}}},
					{Name: "accounts_active_idx", Where: "active = 1", Parts: []schema.IndexPart{{Column: "tenant_id"}}},
					{Name: "accounts_email_lower_key", Unique: true, Parts: []schema.IndexPart{{Expr: "lower(email)"}}},
				},
			},
			{
				Name: "memberships",
				Columns: []schema.Column{
					{Name: "account_id", Type: "INTEGER"},
					{Name: "project_id", Type: "INTEGER"},
					{Name: "role", Type: "TEXT", Default: "'member'"},
				},
				PrimaryKey: []string{"account_id", "project_id"},
				ForeignKeys: []schema.ForeignKey{{
					Name: "memberships_account_id_fkey", Columns: []string{"account_id"}, RefTable: "accounts", RefColumns: []string{"id"},
				}},
			},
			{
				Name: "profiles",
				Columns: []schema.Column{
					{Name: "id", Type: "INTEGER", Identity: "by_default"},
					{Name: "account_id", Type: "INTEGER"},
					{Name: "bio", Type: "TEXT", Nullable: true},
				},
				PrimaryKey: []string{"id"},
				Uniques:    []schema.Unique{{Name: "profiles_account_id_key", Columns: []string{"account_id"}}},
				ForeignKeys: []schema.ForeignKey{{
					Name: "profiles_account_id_fkey", Columns: []string{"account_id"}, RefTable: "accounts", RefColumns: []string{"id"}, OnDelete: "CASCADE",
				}},
			},
			{
				Name: "settings", WithoutRowID: true, Strict: true,
				Columns:    []schema.Column{{Name: "k", Type: "TEXT"}, {Name: "v", Type: "TEXT", Nullable: true}},
				PrimaryKey: []string{"k"},
			},
			{
				Name: "tenants", Autoincrement: true,
				Columns: []schema.Column{
					{Name: "id", Type: "INTEGER", Identity: "by_default"},
					{Name: "name", Type: "TEXT"},
				},
				PrimaryKey: []string{"id"},
				Uniques:    []schema.Unique{{Name: "tenants_name_key", Columns: []string{"name"}}},
			},
		},
		Views:    []schema.View{{Name: "active_accounts", Query: "SELECT id, email FROM accounts WHERE active = 1", Depends: []string{"accounts"}}},
		Triggers: []schema.Trigger{{Name: "accounts_ai", Table: "accounts", Definition: "CREATE TRIGGER accounts_ai AFTER INSERT ON accounts BEGIN SELECT 1; END"}},
	}
}

// TestGenerateFromDBSQLiteRoundTrip writes the models of an inspected SQLite
// schema into a module, loads them with the real generator under the sqlite
// dialect and checks that the schema they declare is the schema the init
// snapshot is written from.
//
// Catches: a SQLite feature that --from-db writes in a form the generator reads
// back differently (the table options, the enum, a generated column, a unique
// index, the rowid alias), so a database adopted from SQLite drifts from its own
// models.
func TestGenerateFromDBSQLiteRoundTrip(t *testing.T) {
	res, err := fromdb.Generate(sqliteShopSchema(), fromdb.Options{
		Package: "models", Dialect: dialect.SQLite, Config: fromdb.Config{Inverse: true},
	})
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
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\npackages:\n  - ./models\ndialects: [sqlite]\n")
	chdir(t, root)

	var out, errb bytes.Buffer
	if code := Run([]string{"generate"}, &out, &errb); code != 0 {
		t.Fatalf("generate: exit %d\n%s%s", code, out.String(), errb.String())
	}
	g, diags, err := gen.Load([]string{"./models"}, gen.Config{
		Dir: root, Naming: naming.Momo{}, Dialects: []dialect.Name{dialect.SQLite},
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

// Catches: --from-db treating a SQLite path as the name of an environment
// variable, or opening a missing file and reading it as an empty database.
func TestGenerateFromDBSQLiteMissingFile(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\npackages:\n  - ./models\ndialects: [sqlite]\n")
	missing := filepath.Join(root, "missing.db")
	for _, src := range []string{
		missing, "missing.db", "sqlite://" + missing, "file:" + missing, "file://" + missing,
		"file:" + missing + "?mode=ro", "file:missing.db", "file:" + filepath.Join(root, "mis%73ing.db"),
	} {
		var out, errb bytes.Buffer
		if code := Run([]string{"generate", "--from-db", src}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "does not exist") {
			t.Fatalf("%s: exit %d, %q", src, code, errb.String())
		}
	}
	for _, src := range []string{":memory:", "file::memory:", "file:x.db?mode=memory", "sqlite://:memory:"} {
		var out, errb bytes.Buffer
		if code := Run([]string{"generate", "--from-db", src}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "in-memory") {
			t.Fatalf("%s: exit %d, %q", src, code, errb.String())
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "*.db")); len(matches) != 0 {
		t.Fatalf("a refused run created a database file: %v", matches)
	}
	if _, err := os.Stat(filepath.Join(root, "models")); err == nil {
		t.Fatal("a refused run created the model directory")
	}
}
