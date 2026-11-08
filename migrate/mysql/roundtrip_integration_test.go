// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

// writeMigration writes the files that a Diff returned into dir.
func writeMigration(t *testing.T, dir string, m *migrate.Migration) {
	t.Helper()
	for name, body := range m.Files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// exampleSchema holds an auto-increment key, a unique key, a composite foreign
// key with ON DELETE CASCADE, a generated column, a CHECK, an index on the
// generated column, defaults, a comment and a JSON column (parents, children),
// then unsigned, boolean, enum, set and collated columns, the default
// spellings the servers rewrite, a CHECK with and, or and in, and DESC, prefix,
// unique prefix, fulltext and (MySQL only) functional indexes (items), and
// foreign keys with SET NULL, RESTRICT and ON UPDATE (links).
//
// MySQL refuses a CHECK or a stored generated column on a column that a
// foreign key with a referential action uses, so qty and price carry them and
// pa and pb carry the key. MariaDB has no functional key parts, so the
// functional index is declared for MySQL only.
func exampleSchema(mariadb bool) *schema.Schema {
	items := schema.Table{
		Name:       "items",
		PrimaryKey: []string{"id"},
		Columns: []schema.Column{
			{Name: "id", Type: "bigint unsigned", Identity: "by_default"},
			{Name: "active", Type: "tinyint(1)", Default: "true"},
			{Name: "level", Type: "tinyint unsigned", Default: "0"},
			{Name: "kind", Type: "enum('a','b')", Default: "'a'"},
			{Name: "flags", Type: "set('x','y')", Nullable: true},
			{Name: "code", Type: "varchar(20)", Collate: "utf8mb4_bin"},
			{Name: "title", Type: "varchar(100)"},
			{Name: "slug", Type: "varchar(100)"},
			{Name: "body", Type: "text", Nullable: true},
			{Name: "stamp1", Type: "datetime", Default: "CURRENT_TIMESTAMP"},
			{Name: "stamp2", Type: "datetime", Default: "NOW()"},
			{Name: "token", Type: "varchar(36)", Default: "uuid()"},
		},
		Checks: []schema.Check{{Name: "items_level", Expr: "level >= 0 and (level < 10 or level in (20,30))"}},
		Indexes: []schema.Index{
			{Name: "items_title_desc", Parts: []schema.IndexPart{{Column: "title", Desc: true}}},
			{Name: "items_body_prefix", Parts: []schema.IndexPart{{Column: "body(10)"}}},
			{Name: "items_slug_prefix", Unique: true, Parts: []schema.IndexPart{{Column: "slug(20)"}}},
			{Name: "items_ft", Method: "fulltext", Parts: []schema.IndexPart{{Column: "title"}, {Column: "body"}}},
		},
	}
	if !mariadb {
		items.Indexes = append(items.Indexes, schema.Index{Name: "items_lower_title", Parts: []schema.IndexPart{{Expr: "lower(title)"}}})
	}
	links := schema.Table{
		Name:       "links",
		PrimaryKey: []string{"id"},
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "parent_id", Type: "bigint unsigned", Nullable: true},
			{Name: "other_id", Type: "bigint unsigned"},
		},
		ForeignKeys: []schema.ForeignKey{
			{Name: "links_parent", Columns: []string{"parent_id"}, RefTable: "items", RefColumns: []string{"id"}, OnDelete: "SET NULL", OnUpdate: "CASCADE"},
			{Name: "links_other", Columns: []string{"other_id"}, RefTable: "items", RefColumns: []string{"id"}, OnDelete: "RESTRICT"},
		},
	}
	return &schema.Schema{
		Dialect: "mysql",
		Tables: []schema.Table{
			{
				Name:       "parents",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "a", Type: "int"},
					{Name: "b", Type: "int"},
				},
				Uniques: []schema.Unique{{Name: "parents_ab", Columns: []string{"a", "b"}}},
			},
			{
				Name:       "children",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "pa", Type: "int"},
					{Name: "pb", Type: "int"},
					{Name: "qty", Type: "int", Default: "0"},
					{Name: "price", Type: "int"},
					{Name: "total", Type: "int", Nullable: true, Generated: "qty * price"},
					{Name: "label", Type: "varchar(40)", Nullable: true, Default: "'none'", Comment: "display label"},
					{Name: "attrs", Type: "json", Nullable: true},
					{Name: "created", Type: "datetime(6)", Default: "CURRENT_TIMESTAMP(6)"},
				},
				ForeignKeys: []schema.ForeignKey{{
					Name:       "children_parent",
					Columns:    []string{"pa", "pb"},
					RefTable:   "parents",
					RefColumns: []string{"a", "b"},
					OnDelete:   "CASCADE",
				}},
				Checks:  []schema.Check{{Name: "qty_nonnegative", Expr: "qty >= 0"}},
				Indexes: []schema.Index{{Name: "children_total", Parts: []schema.IndexPart{{Column: "total"}}}},
			},
			items,
			links,
		},
	}
}

type target struct {
	name    string
	mariadb bool
	open    func(*testing.T, ...momo.Option) *momo.DB
}

var targets = []target{
	{"mysql-8.4", false, testopen.MySQL84},
	{"mysql-9.7", false, testopen.MySQL97},
	{"mariadb-11.8", true, testopen.MariaDB},
}

// applyExample diffs desired into dir, applies it and returns the engine.
func applyExample(t *testing.T, dir string, eng *migrate.Engine, desired *schema.Schema, name string) {
	t.Helper()
	ctx := context.Background()
	m, err := eng.Diff(ctx, desired, name, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatalf("the diff %q produced no migration", name)
	}
	writeMigration(t, dir, m)
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}

// assertLive fails unless the live database equals want and shows no drift.
func assertLive(t *testing.T, eng *migrate.Engine, want *schema.Schema) {
	t.Helper()
	ctx := context.Background()
	live, err := eng.Inspect(ctx)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !schema.Equal(live, want) {
		t.Fatalf("the inspected schema differs from the declared one:\n%s", migratetest.Diff(live, want))
	}
	drift, err := eng.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !drift.Clean() {
		t.Fatalf("drift after apply: %+v", drift)
	}
	// A diff of the declared schema against what the database holds is empty:
	// nothing is left to migrate.
	changes, _, err := migrate.Diff(live, want, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("a diff against the live schema produced changes: %+v", changes)
	}
}

func TestMySQLMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			db := tg.open(t)
			dir := t.TempDir()
			eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
			applyExample(t, dir, eng, exampleSchema(tg.mariadb), "example")
			assertLive(t, eng, exampleSchema(tg.mariadb))
			again, err := eng.Diff(ctx, exampleSchema(tg.mariadb), "noop", migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if again != nil {
				t.Fatalf("a second Engine.Diff produced a migration: %+v", again.Changes)
			}
		})
	}
}

// TestMySQLDropForeignKey: MySQL builds an index for a foreign key and keeps it
// when the key is dropped. The drop must take it along, or the database would
// need a second migration to match the schema.
func TestMySQLDropForeignKey(t *testing.T) {
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			db := tg.open(t)
			dir := t.TempDir()
			eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
			applyExample(t, dir, eng, exampleSchema(tg.mariadb), "example")

			without := exampleSchema(tg.mariadb)
			for i := range without.Tables {
				if without.Tables[i].Name == "links" || without.Tables[i].Name == "children" {
					without.Tables[i].ForeignKeys = nil
				}
			}
			applyExample(t, dir, eng, without, "drop_keys")
			assertLive(t, eng, without)
		})
	}
}
