// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/fromdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"
)

func checkInDialect(d dialect.Name) migrate.Dialect {
	switch d {
	case dialect.SQLite:
		return msqlite.New()
	case dialect.MySQL:
		return mmysql.New()
	}
	return mpg.New()
}

// checkInTable is the notes table of dialect d with a value list over kind.
func checkInTable(d dialect.Name, values ...string) *schema.Schema {
	idType, kindType := "bigint", "text"
	switch d {
	case dialect.SQLite:
		idType, kindType = "INTEGER", "TEXT"
	case dialect.MySQL:
		kindType = "varchar(40)"
	}
	// The test database has vector installed and inspection reports every
	// extension, so the desired PostgreSQL schema declares it.
	var extensions []schema.Extension
	if d == dialect.Postgres {
		extensions = []schema.Extension{{Name: "vector"}}
	}
	return &schema.Schema{Dialect: d, Extensions: extensions, Tables: []schema.Table{{
		Name:       "notes",
		Columns:    []schema.Column{{Name: "id", Type: idType}, {Name: "kind", Type: kindType}},
		PrimaryKey: []string{"id"},
		Checks: []schema.Check{{
			Name: "notes_kind_check",
			Expr: schema.CheckInExpr(d, "kind", values),
			In:   &schema.CheckIn{Column: "kind", Values: slices.Clone(values)},
		}},
	}}}
}

// checkInRun is one database that migrations are written to and applied from.
type checkInRun struct {
	t     *testing.T
	db    *momo.DB
	d     dialect.Name
	files map[string][]byte
	at    time.Time
}

func (r *checkInRun) engine() *migrate.Engine {
	r.at = r.at.Add(24 * time.Hour)
	at := r.at
	return migratetest.NewEngine(r.t, checkInDialect(r.d), r.db.Driver(), migratetest.FS(r.files), migrate.WithClock(func() time.Time { return at }))
}

// migrate diffs the directory against desired, keeps the files and applies them.
func (r *checkInRun) migrate(desired *schema.Schema, name string, opts migrate.DiffOptions) *migrate.Migration {
	r.t.Helper()
	ctx := context.Background()
	m, err := r.engine().Diff(ctx, desired, name, opts)
	if err != nil || m == nil {
		r.t.Fatalf("diff %s: %v, %v", name, m, err)
	}
	for k, v := range m.Files {
		r.files[k] = v
	}
	if _, err := r.engine().Apply(ctx, migrate.ApplyOptions{}); err != nil {
		r.t.Fatalf("apply %s: %v", name, err)
	}
	return m
}

func (r *checkInRun) exec(sql string) error {
	_, err := r.db.Driver().Exec(context.Background(), sql, nil)
	return err
}

func (r *checkInRun) insert(id int, kind string) error {
	return r.exec(fmt.Sprintf("INSERT INTO notes (id, kind) VALUES (%d, '%s')", id, kind))
}

func (r *checkInRun) mustAccept(id int, kind string) {
	r.t.Helper()
	if err := r.insert(id, kind); err != nil {
		r.t.Fatalf("the value %q must be accepted: %v", kind, err)
	}
}

func (r *checkInRun) mustReject(id int, kind string) {
	r.t.Helper()
	if err := r.insert(id, kind); err == nil {
		r.t.Fatalf("the value %q must be rejected by the constraint", kind)
	}
}

func (r *checkInRun) mustBeClean() {
	r.t.Helper()
	rep, err := r.engine().Drift(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	if !rep.Clean() {
		live, _ := r.engine().Inspect(context.Background())
		var checks []schema.Check
		if live != nil && len(live.Tables) > 0 {
			checks = live.Tables[0].Checks
		}
		r.t.Fatalf("drift: %+v\nlive checks: %+v", rep, checks)
	}
}

func (r *checkInRun) onlyChange(m *migrate.Migration, added, removed []string) {
	r.t.Helper()
	if len(m.Changes) != 1 || m.Changes[0].Kind != migrate.AlterCheckIn {
		r.t.Fatalf("changes = %+v, want one alter_check_in", m.Changes)
	}
	c := m.Changes[0]
	if !slices.Equal(c.Added, added) || !slices.Equal(c.Removed, removed) {
		r.t.Fatalf("added %v removed %v, want %v and %v", c.Added, c.Removed, added, removed)
	}
}

// Catches, on every dialect: a value list that a diff replaces instead of
// widening, a widened list that the database does not enforce, an order of the
// declared values that produces a migration, two additions that do not merge
// into one migration, a removal that needs no opt-in, drift between the
// snapshot and what the server stores, and a live list that --from-db does not
// write as a CheckIn declaration or that disagrees with the models it builds.
func TestCheckInRoundTrip(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		r := &checkInRun{t: t, db: db, d: d, files: map[string][]byte{}, at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

		r.migrate(checkInTable(d, "a", "b"), "init", migrate.DiffOptions{})
		r.mustAccept(1, "a")
		r.mustReject(2, "c")
		r.mustBeClean()

		// Add a value: one migration, applied, enforced.
		r.onlyChange(r.migrate(checkInTable(d, "c", "a", "b"), "add_c", migrate.DiffOptions{}), []string{"c"}, nil)
		r.mustAccept(3, "c")
		r.mustReject(4, "zzz")
		r.mustBeClean()

		// The same set in another order is nothing to migrate.
		for _, values := range [][]string{{"a", "b", "c"}, {"c", "b", "a"}, {"b", "a", "c", "a"}} {
			again, err := r.engine().Diff(ctx, checkInTable(d, values...), "noop", migrate.DiffOptions{})
			if err != nil || again != nil {
				t.Fatalf("a second diff of %v must be empty: %v, %v", values, again, err)
			}
		}

		// Two branches each add a value; the merged declaration is one migration.
		merged := r.migrate(checkInTable(d, "a", "b", "c", "e", "d"), "merge_branches", migrate.DiffOptions{})
		r.onlyChange(merged, []string{"d", "e"}, nil)
		r.mustAccept(5, "d")
		r.mustAccept(6, "e")
		r.mustBeClean()

		// Removing a value needs the destructive opt-in and rows to be backfilled first.
		if _, err := r.engine().Diff(ctx, checkInTable(d, "a", "b", "c", "d"), "drop_e", migrate.DiffOptions{}); !errors.Is(err, migrate.ErrDestructive) {
			t.Fatalf("a removed value must need --allow-destructive, got %v", err)
		}
		if err := r.exec("DELETE FROM notes WHERE kind = 'e'"); err != nil {
			t.Fatal(err)
		}
		r.onlyChange(r.migrate(checkInTable(d, "a", "b", "c", "d"), "drop_e", migrate.DiffOptions{AllowDestructive: true}), nil, []string{"e"})
		r.mustReject(7, "e")
		r.mustAccept(8, "d")
		r.mustBeClean()

		// generate --from-db writes the live list as a declaration, and the models
		// it builds agree with the snapshot it writes.
		live, err := r.engine().Inspect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		res, err := fromdb.Generate(live, fromdb.Options{Package: "models", Dialect: d, Flavor: db.Driver().Dialect().Flavor})
		if err != nil {
			t.Fatal(err)
		}
		var src string
		for _, f := range res.Files {
			if f.Table == "notes" {
				src = string(f.Content)
			}
		}
		if want := "c.CheckIn(&m.Kind,\n\t\t\"a\",\n\t\t\"b\",\n\t\t\"c\",\n\t\t\"d\",\n\t)"; !strings.Contains(src, want) {
			t.Fatalf("the model lacks %q:\n%s", want, src)
		}
		changes, _, err := migrate.Diff(res.Snapshot, checkInTable(d, "d", "c", "b", "a"), migrate.DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range changes {
			if c.Kind == migrate.AddCheck || c.Kind == migrate.DropCheck || c.Kind == migrate.AlterCheckIn {
				t.Errorf("the snapshot of --from-db differs from the declared list: %+v", c)
			}
		}
	})
}
