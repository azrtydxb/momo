// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func virtualSchema(virtualSlug bool) *schema.Schema {
	return &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{{
		Name: "vg",
		Columns: []schema.Column{
			{Name: "id", Type: "INTEGER", Identity: "by_default"},
			{Name: "title", Type: "TEXT"},
			{Name: "slug", Type: "TEXT", Nullable: true, Generated: "lower(title)", GeneratedVirtual: virtualSlug},
			{Name: "shout", Type: "TEXT", Nullable: true, Generated: "upper(title)"},
		},
		PrimaryKey: []string{"id"},
	}}}
}

// hiddenOf reads pragma_table_xinfo.hidden of a column: 2 is VIRTUAL, 3 STORED.
func hiddenOf(t *testing.T, c momo.PinnedConn, column string) int {
	t.Helper()
	var h int
	scanOne(t, c, `SELECT hidden FROM pragma_table_xinfo('vg') WHERE name = '`+column+`'`, &h)
	return h
}

// TestSQLiteVirtualGeneratedRoundTrips: a VIRTUAL generated column is inspected
// as VIRTUAL, diffs clean against its declaration, and a fresh migrate creates
// a VIRTUAL column (not a STORED one).
func TestSQLiteVirtualGeneratedRoundTrips(t *testing.T) {
	ctx := context.Background()

	live := pinned(t, testopen.SQLite(t))
	mustExec(t, live, `CREATE TABLE vg (id INTEGER PRIMARY KEY, title TEXT NOT NULL, slug TEXT GENERATED ALWAYS AS (lower(title)) VIRTUAL, shout TEXT GENERATED ALWAYS AS (upper(title)) STORED)`)
	got, err := msqlite.New().Inspect(ctx, live, migrate.InspectOptions{})
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

	fresh := pinned(t, testopen.SQLite(t))
	apply(t, fresh, render(t, &schema.Schema{Dialect: dialect.SQLite}, virtualSchema(true)))
	if h := hiddenOf(t, fresh, "slug"); h != 2 {
		t.Errorf("fresh migrate: slug hidden = %d, want 2 (VIRTUAL)", h)
	}
	if h := hiddenOf(t, fresh, "shout"); h != 3 {
		t.Errorf("fresh migrate: shout hidden = %d, want 3 (STORED)", h)
	}
	assertNoDiff(t, fresh, virtualSchema(true))
}
