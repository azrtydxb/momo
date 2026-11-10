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
