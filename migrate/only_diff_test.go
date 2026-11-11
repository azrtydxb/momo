// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func onlySchema(tables ...string) *schema.Schema {
	s := &schema.Schema{Dialect: "postgres"}
	for _, n := range tables {
		s.Tables = append(s.Tables, schema.Table{
			Name: n, Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"},
		})
	}
	return s
}

// Catches: Engine.Diff ignoring Config.OnlyTables, so a snapshot that holds a
// table outside the list (its model removed) plans DROP TABLE for it, and
// --allow-destructive would run it.
func TestDiffOnlyTablesNeverDropsAnUnmanagedTable(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	first, err := offlineEngine(t, nil, at).Diff(ctx, onlySchema("a", "b", "c"), "init", migrate.DiffOptions{})
	if err != nil || first == nil {
		t.Fatalf("init: %v %v", first, err)
	}
	dir := mapFS(first.Files)
	later := at.Add(time.Hour)
	e := migratetest.NewEngine(t, mpg.New(), nil, dir, migrate.WithClock(func() time.Time { return later }),
		migrate.WithConfig(migrate.Config{OnlyTables: []string{"a", "b"}}))
	for _, destructive := range []bool{false, true} {
		m, err := e.Diff(ctx, onlySchema("a", "b"), "next", migrate.DiffOptions{AllowDestructive: destructive})
		if err != nil || m != nil {
			t.Fatalf("allow destructive %v: want no migration, got %v, %v", destructive, m, err)
		}
	}
	// A real change to a managed table still plans, and never touches c.
	changed := onlySchema("a", "b")
	changed.Tables[0].Columns = append(changed.Tables[0].Columns, schema.Column{Name: "note", Type: "TEXT", Nullable: true})
	m, err := e.Diff(ctx, changed, "note", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil {
		t.Fatalf("diff = %v, %v", m, err)
	}
	for _, s := range m.Up {
		if strings.Contains(s.SQL, `"c"`) || strings.HasPrefix(s.SQL, "DROP") {
			t.Fatalf("unexpected statement %q", s.SQL)
		}
	}
}

// Catches: the desired side of Engine.Diff not being filtered, so a model for
// a table outside Config.OnlyTables (its migration would not be managed here)
// plans a CREATE TABLE for it. A model for such a table is never migrated.
func TestDiffOnlyTablesNeverCreatesAnUnmanagedTable(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	first, err := offlineEngine(t, nil, at).Diff(ctx, onlySchema("a", "b"), "init", migrate.DiffOptions{})
	if err != nil || first == nil {
		t.Fatalf("init: %v %v", first, err)
	}
	later := at.Add(time.Hour)
	e := migratetest.NewEngine(t, mpg.New(), nil, mapFS(first.Files), migrate.WithClock(func() time.Time { return later }),
		migrate.WithConfig(migrate.Config{OnlyTables: []string{"a", "b"}}))
	m, err := e.Diff(ctx, onlySchema("a", "b", "c"), "next", migrate.DiffOptions{})
	if err != nil || m != nil {
		t.Fatalf("a model for a table outside only must plan nothing, got %v, %v", m, err)
	}
}

// Catches: a malformed or empty OnlyTables entry reaching Diff unchecked: a
// bad glob named Config.Exclude in the error, and an empty entry silently
// hiding every table.
func TestDiffRejectsBadOnlyTables(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, globs := range map[string][]string{"bad glob": {"["}, "empty entry": {""}, "blank entry": {"a", "  "}} {
		e := migratetest.NewEngine(t, mpg.New(), nil, mapFS(nil), migrate.WithClock(func() time.Time { return at }),
			migrate.WithConfig(migrate.Config{OnlyTables: globs}))
		_, err := e.Diff(context.Background(), onlySchema("a"), "init", migrate.DiffOptions{})
		if err == nil || !strings.Contains(err.Error(), "Config.OnlyTables") || strings.Contains(err.Error(), "Config.Exclude") {
			t.Errorf("%s: err = %v, want an error that names Config.OnlyTables", name, err)
		}
	}
}
