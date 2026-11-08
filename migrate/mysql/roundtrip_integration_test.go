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
// generated column, defaults, a comment and a JSON column.
//
// MySQL refuses a CHECK or a stored generated column on a column that a
// foreign key with a referential action uses, so qty and price carry them and
// pa and pb carry the key.
func exampleSchema() *schema.Schema {
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
		},
	}
}

func TestMySQLMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, target := range []struct {
		name string
		open func(*testing.T, ...momo.Option) *momo.DB
	}{
		{"mysql-8.4", testopen.MySQL84},
		{"mysql-9.7", testopen.MySQL97},
		{"mariadb-11.8", testopen.MariaDB},
	} {
		t.Run(target.name, func(t *testing.T) {
			db := target.open(t)
			dir := t.TempDir()
			eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
			m, err := eng.Diff(ctx, exampleSchema(), "example", migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if m == nil {
				t.Fatal("the first diff produced no migration")
			}
			writeMigration(t, dir, m)
			if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
				t.Fatalf("apply: %v", err)
			}
			live, err := eng.Inspect(ctx)
			if err != nil {
				t.Fatalf("inspect: %v", err)
			}
			if !schema.Equal(live, exampleSchema()) {
				t.Fatalf("the inspected schema differs from the declared one:\n%s", migratetest.Diff(live, exampleSchema()))
			}
			drift, err := eng.Drift(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !drift.Clean() {
				t.Fatalf("drift after apply: %+v", drift)
			}
			// A diff of the declared schema against what the database holds is
			// empty: nothing is left to migrate.
			changes, _, err := migrate.Diff(live, exampleSchema(), migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(changes) != 0 {
				t.Fatalf("a second diff produced changes: %+v", changes)
			}
			again, err := eng.Diff(ctx, exampleSchema(), "noop", migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if again != nil {
				t.Fatalf("a second Engine.Diff produced a migration: %+v", again.Changes)
			}
		})
	}
}
