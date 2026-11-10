// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func engineAt(t *testing.T, db *momo.DB, files map[string][]byte, day int) *migrate.Engine {
	t.Helper()
	when := time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
	return migratetest.NewEngine(t, mpg.New(), db.Driver(), withMigrations(files), migrate.WithClock(func() time.Time { return when }))
}

// mergeFiles returns the union of the file maps; a later map wins.
func mergeFiles(maps ...map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// TestStagedChange runs the staged NOT NULL flow on a populated table: add the
// column nullable, backfill with a data statement, then SET NOT NULL, which
// applies because every row satisfies it.
//
// Catches: a diff that emits NOT NULL directly for a populated table without a
// default, which fails on the first row, and a follow-up that never enforces.
func TestStagedChange(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	m1, err := engineAt(t, db, nil, 1).Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatalf("init: %v %v", m1, err)
	}
	if _, err := engineAt(t, db, m1.Files, 1).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	mustExecDB(t, db, `INSERT INTO docs (id) VALUES (1), (2), (3)`)

	desired := docsSchema()
	desired.Tables[0].Columns = append(desired.Tables[0].Columns, schema.Column{Name: "tenant", Type: "BIGINT"})

	// A NOT NULL column without a default is refused outright.
	if _, err := engineAt(t, db, m1.Files, 2).Diff(ctx, desired, "tenant", migrate.DiffOptions{}); !errors.Is(err, migrate.ErrStagedRequired) {
		t.Fatalf("a NOT NULL column without a default must need staging, got %v", err)
	}

	// Stage: the column is added nullable and applies to the populated table.
	m2, err := engineAt(t, db, m1.Files, 2).Diff(ctx, desired, "tenant", migrate.DiffOptions{Stage: true})
	if err != nil || m2 == nil {
		t.Fatalf("stage: %v %v", m2, err)
	}
	staged := mergeFiles(m1.Files, m2.Files)
	if _, err := engineAt(t, db, staged, 2).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply the staged add: %v", err)
	}
	if n := firstValue(t, db, `SELECT count(*)::text FROM docs WHERE tenant IS NULL`); n != "3" {
		t.Fatalf("the staged column is nullable and empty, NULL rows = %s", n)
	}

	// Backfill, then enforce.
	mustExecDB(t, db, `UPDATE docs SET tenant = id`)
	m3, err := engineAt(t, db, staged, 3).Diff(ctx, desired, "not_null", migrate.DiffOptions{EnforceNotNull: true})
	if err != nil || m3 == nil {
		t.Fatalf("enforce: %v %v", m3, err)
	}
	if up := upOf(m3); !strings.Contains(up, `"tenant" SET NOT NULL`) {
		t.Fatalf("the follow-up must emit SET NOT NULL:\n%s", up)
	}
	enforced := mergeFiles(staged, m3.Files)
	if _, err := engineAt(t, db, enforced, 3).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply SET NOT NULL on the backfilled table: %v", err)
	}

	if n := firstValue(t, db, `SELECT count(*)::text FROM docs WHERE tenant IS NULL`); n != "0" {
		t.Fatalf("every row must satisfy the constraint, NULL rows = %s", n)
	}
	if v := firstValue(t, db, `SELECT is_nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'docs' AND column_name = 'tenant'`); v != "NO" {
		t.Fatalf("the column must be NOT NULL in the database, is_nullable = %s", v)
	}
	if err := execDB(t, db, `INSERT INTO docs (id) VALUES (9)`); err == nil {
		t.Fatal("an insert without tenant must now fail")
	}
	if again, err := engineAt(t, db, enforced, 4).Diff(ctx, desired, "noop", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("models and snapshot agree after the follow-up: %v %v", again, err)
	}
}

// TestCustomMigrationDriftMismatch: SQL a hand-written migration runs against a
// modeled table is invisible to Diff, which trusts the snapshot (a custom
// migration copies its parent's), and Drift reports the column the models do
// not describe.
//
// Catches: a drift that only compares the snapshot with the models, or a diff
// that re-inspects the database and silently drops the custom column.
func TestCustomMigrationDriftMismatch(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	m1, err := engineAt(t, db, nil, 1).Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatalf("init: %v %v", m1, err)
	}
	dir := withMigrations(m1.Files)
	customWith(t, dir, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "addcol", "ALTER TABLE docs ADD COLUMN extra TEXT;\n")
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), dir)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	again, err := e.Diff(ctx, docsSchema(), "noop", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("the diff trusts the snapshot and must see no change: %v %v", again, err)
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean() {
		t.Fatal("a column added by custom SQL that the models do not describe must be reported by drift")
	}
	found := false
	for _, c := range rep.Changed {
		if c.Object.Kind == "column" && c.Object.Name == "docs.extra" && strings.Contains(c.Detail, "not in snapshot") {
			found = true
		}
	}
	if !found {
		t.Fatalf("drift must name the column docs.extra as not in the snapshot: %+v", rep)
	}
}
