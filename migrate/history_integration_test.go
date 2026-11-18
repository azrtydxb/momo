// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
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

// notesHistorySchema is the schema the generator builds for a notes model with
// Config.History and the given columns.
func notesHistorySchema(t *testing.T, cols ...schema.Column) *schema.Schema {
	t.Helper()
	tbl := schema.Table{Name: "notes", Columns: cols, PrimaryKey: []string{"id"}}
	h, err := schema.HistoryFor(&tbl)
	if err != nil {
		t.Fatal(err)
	}
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables:     []schema.Table{tbl, h.Table},
		Functions:  []schema.Function{h.Function},
		Triggers:   []schema.Trigger{h.Trigger},
	}
}

// Catches: history objects that the engine creates but inspection reads back
// differently (a function body, a trigger event list, an index, an identity
// column), which would show as drift on every database that uses history, and
// make every migrate diff write a migration.
func TestHistoryObjectsRoundTripWithoutDrift(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := notesHistorySchema(t,
		schema.Column{Name: "id", Type: "bigint"},
		schema.Column{Name: "title", Type: "text"},
		schema.Column{Name: "body", Type: "text", Nullable: true},
	)
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, desired, "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	inspected, err := e.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Equal(inspected, desired) {
		t.Fatalf("round trip not empty\n%s", migratetest.Diff(inspected, desired))
	}
	again, err := e.Diff(ctx, desired, "noop", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("no-change diff must write nothing: %v %v", again, err)
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("drift after applying the history objects: changed %v, missing %v, unmanaged %v", rep.Changed, rep.Missing, rep.Unmanaged)
	}
}

// Catches: a column added to or dropped from the model in a plan that does not
// repeat it on the history table and in the trigger function: the next write to
// the audited table would fail, or keep recording a column that is gone.
func TestColumnChangesReachTheHistoryTableInOnePlan(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	v1 := notesHistorySchema(t,
		schema.Column{Name: "id", Type: "bigint"},
		schema.Column{Name: "title", Type: "text"},
		schema.Column{Name: "body", Type: "text", Nullable: true},
	)
	at := func(day int) migrate.Option {
		return migrate.WithClock(func() time.Time { return time.Date(2026, 1, day, 12, 0, 0, 0, time.UTC) })
	}
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS(), at(1))
	first, err := m.Diff(ctx, v1, "init", migrate.DiffOptions{})
	if err != nil || first == nil {
		t.Fatalf("diff: %v %v", first, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(first.Files), at(2))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO notes (id, title, body) VALUES (1, 'old', 'gone soon')"); err != nil {
		t.Fatal(err)
	}

	v2 := notesHistorySchema(t,
		schema.Column{Name: "id", Type: "bigint"},
		schema.Column{Name: "title", Type: "text"},
		schema.Column{Name: "rank", Type: "integer", Default: "0"},
	)
	second, err := e.Diff(ctx, v2, "columns", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || second == nil {
		t.Fatalf("diff of the column change: %v %v", second, err)
	}
	var up strings.Builder
	for name, content := range second.Files {
		if strings.HasSuffix(name, ".up.sql") {
			up.Write(content)
		}
	}
	for _, want := range []string{`ALTER TABLE "notes" ADD COLUMN "rank"`, `ALTER TABLE "notes_history" ADD COLUMN "rank"`, `ALTER TABLE "notes" DROP COLUMN "body"`, `ALTER TABLE "notes_history" DROP COLUMN "body"`, `CREATE OR REPLACE FUNCTION "notes_history_fn"`} {
		if !strings.Contains(up.String(), want) {
			t.Errorf("the plan lacks %s:\n%s", want, up.String())
		}
	}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mergeFiles(first.Files, second.Files)))
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	inspected, err := e2.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Equal(inspected, v2) {
		t.Fatalf("after the column change\n%s", migratetest.Diff(inspected, v2))
	}

	// The trigger keeps recording after the change, with the new column.
	if _, err := momo.Exec(ctx, db, "UPDATE notes SET rank = 5 WHERE id = 1"); err != nil {
		t.Fatalf("a write after the column change: %v", err)
	}
	ranks, err := momo.Scalars[int64](ctx, db, "SELECT rank FROM notes_history WHERE id = 1 ORDER BY history_id DESC LIMIT 1")
	if err != nil || len(ranks) != 1 || ranks[0] != 5 {
		t.Fatalf("the newest history row has rank %v (%v), want 5", ranks, err)
	}
	// The versions recorded before the change are kept.
	n, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM notes_history WHERE id = 1")
	if err != nil || len(n) != 1 || n[0] != 2 {
		t.Fatalf("history rows of the note = %v (%v), want 2 (the insert and the update)", n, err)
	}
}

func mergeFiles(a, b map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
