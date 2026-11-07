// SPDX-License-Identifier: Apache-2.0

package main

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
