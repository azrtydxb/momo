// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

func virtualSchema(virtualSlug bool) *schema.Schema {
	return &schema.Schema{Dialect: dialect.MySQL, Tables: []schema.Table{{
		Name: "vg",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "title", Type: "varchar(100)"},
			{Name: "slug", Type: "bigint", Nullable: true, Generated: "id * 2", GeneratedVirtual: virtualSlug},
			{Name: "shout", Type: "bigint", Nullable: true, Generated: "id * 3"},
		},
		PrimaryKey: []string{"id"},
	}}}
}

// generationExtra reads information_schema.COLUMNS.EXTRA of vg.column.
func generationExtra(t *testing.T, db *momo.DB, column string) string {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(),
		"SELECT EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'vg' AND COLUMN_NAME = ?", []any{column})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("no column %s: %v", column, rows.Err())
	}
	var extra string
	if err := rows.Scan(&extra); err != nil {
		t.Fatal(err)
	}
	return extra
}

// TestMySQLVirtualGeneratedRoundTrips: a VIRTUAL generated column is inspected
// as VIRTUAL, diffs clean against its declaration, and a fresh migrate creates
// a VIRTUAL column (not a STORED one).
func TestMySQLVirtualGeneratedRoundTrips(t *testing.T) {
	ctx := context.Background()
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			live := tg.open(t)
			if _, err := momo.Exec(ctx, live, "CREATE TABLE vg (id bigint NOT NULL, title varchar(100) NOT NULL, slug bigint GENERATED ALWAYS AS (id * 2) VIRTUAL, shout bigint GENERATED ALWAYS AS (id * 3) STORED, PRIMARY KEY (id))"); err != nil {
				t.Fatal(err)
			}
			eng := migratetest.NewEngine(t, mmysql.New(), live.Driver(), os.DirFS(t.TempDir()))
			got, err := eng.Inspect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			tbl, _ := got.Table("vg")
			slug, _ := tbl.Column("slug")
			shout, _ := tbl.Column("shout")
			if !slug.GeneratedVirtual || shout.GeneratedVirtual {
				t.Fatalf("inspected kinds: slug virtual=%v (want true), shout virtual=%v (want false)", slug.GeneratedVirtual, shout.GeneratedVirtual)
			}
			changes, _, err := migrate.Diff(got, virtualSchema(true), migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, ch := range changes {
				t.Errorf("diff of the inspected table against its declaration: %s %s.%s", ch.Kind, ch.Table, ch.Name)
			}
			// Declared STORED while the database holds VIRTUAL is a change.
			changes, _, err = migrate.Diff(got, virtualSchema(false), migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(changes) == 0 {
				t.Error("a VIRTUAL to STORED change produced no diff")
			}

			// The helper returns the one database of the target, so clear it
			// and create the table again from the declaration.
			if _, err := momo.Exec(ctx, live, "DROP TABLE vg"); err != nil {
				t.Fatal(err)
			}
			fresh := live
			dir := t.TempDir()
			feng := migratetest.NewEngine(t, mmysql.New(), fresh.Driver(), os.DirFS(dir))
			applyExample(t, dir, feng, virtualSchema(true), "virtual")
			if x := generationExtra(t, fresh, "slug"); !strings.Contains(strings.ToUpper(x), "VIRTUAL") {
				t.Errorf("fresh migrate: slug EXTRA = %q, want VIRTUAL GENERATED", x)
			}
			if x := generationExtra(t, fresh, "shout"); !strings.Contains(strings.ToUpper(x), "STORED") && !strings.Contains(strings.ToUpper(x), "PERSISTENT") {
				t.Errorf("fresh migrate: shout EXTRA = %q, want STORED GENERATED", x)
			}
			assertLive(t, feng, virtualSchema(true))
		})
	}
}
