// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func snapshotOf(t *testing.T, m *migrate.Migration) *schema.Schema {
	t.Helper()
	for name, b := range m.Files {
		if strings.HasSuffix(name, ".snapshot.json") {
			s, err := migrate.ReadSnapshot(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			return s.Schema
		}
	}
	t.Fatal("migration has no snapshot file")
	return nil
}

func upOf(m *migrate.Migration) string {
	for name, b := range m.Files {
		if strings.HasSuffix(name, ".up.sql") {
			return string(b)
		}
	}
	return ""
}

func findColumn(s *schema.Schema, table, col string) *schema.Column {
	for _, t := range s.Tables {
		if t.Name != table {
			continue
		}
		for i := range t.Columns {
			if t.Columns[i].Name == col {
				return &t.Columns[i]
			}
		}
	}
	return nil
}

// withMigrations returns the base files plus the files of each migration.
func withMigrations(base map[string][]byte, ms ...*migrate.Migration) fstest.MapFS {
	out := mapFS(base)
	for _, m := range ms {
		for k, v := range m.Files {
			out[k] = &fstest.MapFile{Data: v}
		}
	}
	return out
}

// initFiles is the files of the initial docs migration.
func initFiles(t *testing.T) map[string][]byte {
	t.Helper()
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(context.Background(), docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	return m1.Files
}

func TestStagedAddColumnIsEnforcedByTheNextDiff(t *testing.T) {
	ctx := context.Background()
	files := initFiles(t)
	desired := docsSchema()
	desired.Tables[0].Columns = append(desired.Tables[0].Columns, schema.Column{Name: "tenant", Type: "BIGINT"})

	e2 := offlineEngine(t, mapFS(files), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	m2, err := e2.Diff(ctx, desired, "add_tenant", migrate.DiffOptions{Stage: true})
	if err != nil || m2 == nil {
		t.Fatal(m2, err)
	}
	if up := upOf(m2); !strings.Contains(up, "ADD COLUMN") || strings.Contains(up, "NOT NULL") {
		t.Fatalf("the staged add must be nullable:\n%s", up)
	}
	if c := findColumn(snapshotOf(t, m2), "docs", "tenant"); c == nil || !c.Nullable {
		t.Fatalf("the staged snapshot must record tenant as nullable like the database: %+v", c)
	}
	if desired.Tables[0].Columns[2].Nullable {
		t.Fatal("Diff must not modify the desired schema")
	}

	// The follow-up: a plain diff still refuses, the flag emits SET NOT NULL.
	e3 := offlineEngine(t, withMigrations(files, m2), time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if _, err := e3.Diff(ctx, desired, "enforce", migrate.DiffOptions{}); !errors.Is(err, migrate.ErrStagedRequired) {
		t.Fatalf("without the flag the follow-up must refuse, got %v", err)
	}
	m3, err := e3.Diff(ctx, desired, "enforce", migrate.DiffOptions{EnforceNotNull: true})
	if err != nil || m3 == nil {
		t.Fatalf("the follow-up must emit a change: %v %v", m3, err)
	}
	if up := upOf(m3); !strings.Contains(up, `"tenant" SET NOT NULL`) {
		t.Fatalf("follow-up must emit SET NOT NULL:\n%s", up)
	}
	if c := findColumn(snapshotOf(t, m3), "docs", "tenant"); c == nil || c.Nullable {
		t.Fatalf("the follow-up snapshot must record NOT NULL: %+v", c)
	}

	// After the follow-up the models and the snapshot agree.
	e4 := offlineEngine(t, withMigrations(files, m2, m3), time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC))
	if m4, err := e4.Diff(ctx, desired, "noop", migrate.DiffOptions{}); err != nil || m4 != nil {
		t.Fatalf("after the follow-up diff must report no changes: %v %v", m4, err)
	}
}

func TestStagedExistingColumnIsEnforcedByTheNextDiff(t *testing.T) {
	ctx := context.Background()
	files := initFiles(t)
	nullable := docsSchema()
	nullable.Tables[0].Columns[1] = schema.Column{Name: "title", Type: "TEXT", Nullable: true}
	e2 := offlineEngine(t, mapFS(files), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	m2, err := e2.Diff(ctx, nullable, "title_nullable", migrate.DiffOptions{})
	if err != nil || m2 == nil {
		t.Fatal(m2, err)
	}
	notNull := docsSchema()
	notNull.Tables[0].Columns[1] = schema.Column{Name: "title", Type: "TEXT"}
	e3 := offlineEngine(t, withMigrations(files, m2), time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if _, err := e3.Diff(ctx, notNull, "x", migrate.DiffOptions{}); !errors.Is(err, migrate.ErrStagedRequired) {
		t.Fatalf("NULL to NOT NULL must refuse, got %v", err)
	}
	if m, err := e3.Diff(ctx, notNull, "x", migrate.DiffOptions{Stage: true}); err != nil || m != nil {
		t.Fatalf("staging an existing column writes nothing: %v %v", m, err)
	}
	m3, err := e3.Diff(ctx, notNull, "enforce", migrate.DiffOptions{EnforceNotNull: true})
	if err != nil || m3 == nil {
		t.Fatalf("enforce must emit a change: %v %v", m3, err)
	}
	if up := upOf(m3); !strings.Contains(up, `"title" SET NOT NULL`) {
		t.Fatalf("enforce must emit SET NOT NULL:\n%s", up)
	}
	if c := findColumn(snapshotOf(t, m3), "docs", "title"); c == nil || c.Nullable {
		t.Fatalf("snapshot must record NOT NULL: %+v", c)
	}
}

// Two columns staged in two different migrations, with no backfill between
// them, are both enforced by one --enforce-not-null.
func TestStagedColumnsCarriedThroughALaterStageDiff(t *testing.T) {
	ctx := context.Background()
	files := initFiles(t)

	d2 := docsSchema()
	d2.Tables[0].Columns = append(d2.Tables[0].Columns, schema.Column{Name: "tenant", Type: "BIGINT"})
	e2 := offlineEngine(t, mapFS(files), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	m2, err := e2.Diff(ctx, d2, "add_tenant", migrate.DiffOptions{Stage: true})
	if err != nil || m2 == nil {
		t.Fatal(m2, err)
	}

	d3 := docsSchema()
	d3.Tables[0].Columns = append(d3.Tables[0].Columns,
		schema.Column{Name: "tenant", Type: "BIGINT"}, schema.Column{Name: "region", Type: "TEXT"})
	e3 := offlineEngine(t, withMigrations(files, m2), time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	m3, err := e3.Diff(ctx, d3, "add_region", migrate.DiffOptions{Stage: true})
	if err != nil || m3 == nil {
		t.Fatal(m3, err)
	}
	snap3 := snapshotOf(t, m3)
	for _, c := range []string{"tenant", "region"} {
		if col := findColumn(snap3, "docs", c); col == nil || !col.Nullable {
			t.Fatalf("%s is still nullable in the database, the snapshot must say so: %+v", c, col)
		}
	}

	e4 := offlineEngine(t, withMigrations(files, m2, m3), time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC))
	m4, err := e4.Diff(ctx, d3, "enforce", migrate.DiffOptions{EnforceNotNull: true})
	if err != nil || m4 == nil {
		t.Fatalf("enforce: %v %v", m4, err)
	}
	for _, c := range []string{"tenant", "region"} {
		if !strings.Contains(upOf(m4), `"`+c+`" SET NOT NULL`) {
			t.Fatalf("enforce must emit SET NOT NULL for %s:\n%s", c, upOf(m4))
		}
		if col := findColumn(snapshotOf(t, m4), "docs", c); col == nil || col.Nullable {
			t.Fatalf("%s must be NOT NULL after enforce: %+v", c, col)
		}
	}

	e5 := offlineEngine(t, withMigrations(files, m2, m3, m4), time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	if m5, err := e5.Diff(ctx, d3, "noop", migrate.DiffOptions{}); err != nil || m5 != nil {
		t.Fatalf("after enforce diff must report no changes: %v %v", m5, err)
	}
}

func TestEnforceNotNullDoesNotAuthorizeNewColumns(t *testing.T) {
	ctx := context.Background()
	desired := docsSchema()
	desired.Tables[0].Columns = append(desired.Tables[0].Columns, schema.Column{Name: "tenant", Type: "BIGINT"})
	e2 := offlineEngine(t, mapFS(initFiles(t)), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if _, err := e2.Diff(ctx, desired, "x", migrate.DiffOptions{EnforceNotNull: true}); !errors.Is(err, migrate.ErrStagedRequired) {
		t.Fatalf("a NOT NULL add must still need --stage, got %v", err)
	}
}
