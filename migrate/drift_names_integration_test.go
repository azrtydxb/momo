// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

// driftDesired is the items table of dialect d: a NOT NULL status column with
// a default and two value-list checks, the shape of the adoption the issue
// describes (a column whose default and NOT NULL differ, two missing CHECKs).
func driftDesired(d dialect.Name) *schema.Schema {
	idType, textType, intType := "bigint", "text", "integer"
	switch d {
	case dialect.SQLite:
		idType, textType, intType = "INTEGER", "TEXT", "INTEGER"
	case dialect.MySQL:
		textType, intType = "varchar(20)", "int"
	}
	var extensions []schema.Extension
	if d == dialect.Postgres {
		// The test database has vector installed and inspection reports every extension.
		extensions = []schema.Extension{{Name: "vector"}}
	}
	check := func(name, col string, values ...string) schema.Check {
		return schema.Check{Name: name, Expr: schema.CheckInExpr(d, col, values), In: &schema.CheckIn{Column: col, Values: values}}
	}
	return &schema.Schema{Dialect: d, Extensions: extensions, Tables: []schema.Table{{
		Name: "items",
		Columns: []schema.Column{
			{Name: "id", Type: idType},
			{Name: "status", Type: textType, Default: "'new'"},
			{Name: "kind", Type: textType, Nullable: true},
			{Name: "qty", Type: intType, Nullable: true},
		},
		PrimaryKey: []string{"id"},
		Checks: []schema.Check{
			check("items_status_check", "status", "new", "done"),
			check("items_kind_check", "kind", "a", "b"),
		},
	}}}
}

// driftMutations are the statements that make the live items table differ from
// driftDesired the way the issue's database did: status loses its default and
// its NOT NULL, and both checks are gone. SQLite cannot alter a column, so it
// recreates the table.
func driftMutations(d dialect.Name) []string {
	switch d {
	case dialect.SQLite:
		return []string{
			`DROP TABLE items`,
			`CREATE TABLE items (id INTEGER NOT NULL, status TEXT, kind TEXT, qty INTEGER, PRIMARY KEY (id))`,
		}
	case dialect.MySQL:
		return []string{
			`ALTER TABLE items MODIFY status varchar(20) NULL`,
			`ALTER TABLE items DROP CONSTRAINT items_status_check`,
			`ALTER TABLE items DROP CONSTRAINT items_kind_check`,
		}
	}
	return []string{
		`ALTER TABLE items ALTER COLUMN status DROP DEFAULT`,
		`ALTER TABLE items ALTER COLUMN status DROP NOT NULL`,
		`ALTER TABLE items DROP CONSTRAINT items_status_check`,
		`ALTER TABLE items DROP CONSTRAINT items_kind_check`,
	}
}

// driftFixture is a database built from the migrations of driftDesired, then
// changed outside them, then left without momo history: a database another tool
// built that is adopted.
type driftFixture struct {
	t       *testing.T
	db      *momo.DB
	d       dialect.Name
	files   map[string][]byte
	version string
}

func (f *driftFixture) engine() *migrate.Engine {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return migratetest.NewEngine(f.t, checkInDialect(f.d), f.db.Driver(), f.fs(), migrate.WithClock(func() time.Time { return at }))
}

func (f *driftFixture) fs() fs.FS { return migratetest.FS(f.files) }

func (f *driftFixture) exec(sql string) {
	f.t.Helper()
	if _, err := f.db.Driver().Exec(context.Background(), sql, nil); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

// newDriftFixture builds the database from the migrations, mutates it and
// drops the history table.
func newDriftFixture(t *testing.T, db *momo.DB, d dialect.Name) *driftFixture {
	t.Helper()
	ctx := context.Background()
	f := &driftFixture{t: t, db: db, d: d}
	m, err := f.engine().Diff(ctx, driftDesired(d), "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v, %v", m, err)
	}
	f.files, f.version = m.Files, m.Version
	if _, err := f.engine().Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rep, err := f.engine().Drift(ctx); err != nil || !rep.Clean() {
		t.Fatalf("the migrated database must be clean: %+v, %v", rep, err)
	}
	for _, sql := range driftMutations(d) {
		f.exec(sql)
	}
	f.exec(`DROP TABLE momo_migrations`)
	return f
}

// Catches: the adopt refusal giving only counts ("1 changed, 2 missing"), so
// that the differing objects had to be found by diffing two dumps by hand.
func TestBaselineRefusalNamesTheDifferingObjects(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		f := newDriftFixture(t, db, d)
		err := f.engine().Baseline(context.Background(), f.version, false)
		if !errors.Is(err, migrate.ErrBaselineMismatch) {
			t.Fatalf("baseline of a database that differs: %v, want ErrBaselineMismatch", err)
		}
		for _, want := range []string{"items.status", "items.items_status_check", "items.items_kind_check"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not name %q:\n%v", want, err)
			}
		}
	})
}
