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
	for _, want := range []string{`ALTER TABLE "notes" ADD COLUMN "rank"`, `ALTER TABLE "notes_history" ADD COLUMN "rank"`, `ALTER TABLE "notes" DROP COLUMN "body"`, `CREATE OR REPLACE FUNCTION "notes_history_fn"`} {
		if !strings.Contains(up.String(), want) {
			t.Errorf("the plan lacks %s:\n%s", want, up.String())
		}
	}
	// The history table keeps the column the model dropped.
	if strings.Contains(up.String(), `ALTER TABLE "notes_history" DROP COLUMN`) {
		t.Errorf("the plan drops a column of the history table:\n%s", up.String())
	}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mergeFiles(first.Files, second.Files)))
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	// ADD COLUMN appends, so the inspected history table keeps the model
	// columns after its history_ columns; schema.Equal counts column order, so
	// the check is a diff that finds nothing to do.
	if again, err := e2.Diff(ctx, v2, "noop", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("after the column change the diff must write nothing: %v %v", again, err)
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
	// The column the model dropped is still on the history table, with its data.
	bodies, err := momo.Scalars[string](ctx, db, "SELECT body FROM notes_history WHERE id = 1 AND history_operation = 'INSERT'")
	if err != nil || len(bodies) != 1 || bodies[0] != "gone soon" {
		t.Fatalf("the recorded body = %v (%v), want it kept", bodies, err)
	}
	rep, err := e2.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("drift with the retained history column: %+v, %v", rep, err)
	}
}

// evolve diffs desired against the migrations in files, applies the result to
// the database and returns the files with the new migration added.
func evolve(ctx context.Context, t *testing.T, db *momo.DB, files map[string][]byte, desired *schema.Schema, name string, day int, opts migrate.DiffOptions) map[string][]byte {
	t.Helper()
	clock := migrate.WithClock(func() time.Time { return time.Date(2026, 2, day, 12, 0, 0, 0, time.UTC) })
	fsys := migratetest.EmptyFS()
	if files != nil {
		fsys = migratetest.FS(files)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, clock)
	mig, err := e.Diff(ctx, desired, name, opts)
	if err != nil || mig == nil {
		t.Fatalf("diff %s: %v %v", name, mig, err)
	}
	merged := map[string][]byte{}
	for k, v := range files {
		merged[k] = v
	}
	for k, v := range mig.Files {
		merged[k] = v
	}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(merged), clock)
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
	rep, err := e2.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("drift after %s: %+v, %v", name, rep, err)
	}
	if again, err := e2.Diff(ctx, desired, name+"_again", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("a second diff after %s must write nothing: %v %v", name, again, err)
	}
	return merged
}

// Catches: a model column that is dropped from the history table with it (the
// audit data goes), or kept NOT NULL (the trigger no longer writes it, so every
// later write to the model fails), or a retained column that drift reports.
func TestDroppedNotNullColumnKeepsItsHistory(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	files := evolve(ctx, t, db, nil, notesHistorySchema(t,
		schema.Column{Name: "id", Type: "bigint"},
		schema.Column{Name: "title", Type: "text"},
		schema.Column{Name: "body", Type: "text", Nullable: true},
	), "init", 1, migrate.DiffOptions{})
	if _, err := momo.Exec(ctx, db, "INSERT INTO notes (id, title) VALUES (1, 'kept forever')"); err != nil {
		t.Fatal(err)
	}
	evolve(ctx, t, db, files, notesHistorySchema(t,
		schema.Column{Name: "id", Type: "bigint"},
		schema.Column{Name: "body", Type: "text", Nullable: true},
	), "drop_title", 2, migrate.DiffOptions{AllowDestructive: true})

	if _, err := momo.Exec(ctx, db, "INSERT INTO notes (id, body) VALUES (2, 'new')"); err != nil {
		t.Fatalf("a write after the model dropped a NOT NULL column: %v", err)
	}
	titles, err := momo.Scalars[string](ctx, db, "SELECT title FROM notes_history WHERE id = 1")
	if err != nil || len(titles) != 1 || titles[0] != "kept forever" {
		t.Fatalf("the recorded title = %v (%v), want it kept", titles, err)
	}
	nulls, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM notes_history WHERE id = 2 AND title IS NULL")
	if err != nil || len(nulls) != 1 || nulls[0] != 1 {
		t.Fatalf("new history rows with a NULL title = %v (%v), want 1", nulls, err)
	}
}

// Catches: a column rename of the model that drops and re-adds the column on
// the history table, losing every recorded value of it.
func TestColumnRenameKeepsTheRecordedValues(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	files := evolve(ctx, t, db, nil, notesHistorySchema(t,
		schema.Column{Name: "id", Type: "bigint"},
		schema.Column{Name: "title", Type: "text"},
	), "init", 1, migrate.DiffOptions{})
	if _, err := momo.Exec(ctx, db, "INSERT INTO notes (id, title) VALUES (1, 'before')"); err != nil {
		t.Fatal(err)
	}
	evolve(ctx, t, db, files, notesHistorySchema(t,
		schema.Column{Name: "id", Type: "bigint"},
		schema.Column{Name: "headline", Type: "text"},
	), "rename_title", 2, migrate.DiffOptions{RenameColumns: map[string]string{"notes.title": "headline"}})

	got, err := momo.Scalars[string](ctx, db, "SELECT headline FROM notes_history WHERE id = 1")
	if err != nil || len(got) != 1 || got[0] != "before" {
		t.Fatalf("the recorded headline = %v (%v), want the old title", got, err)
	}
	if _, err := momo.Exec(ctx, db, "UPDATE notes SET headline = 'after' WHERE id = 1"); err != nil {
		t.Fatalf("a write after the rename: %v", err)
	}
	if n, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM notes_history WHERE id = 1"); err != nil || n[0] != 2 {
		t.Fatalf("history rows = %v (%v), want 2", n, err)
	}
}

// Catches: a table rename that creates an empty history table under the new
// name and drops the recorded versions, or leaves the trigger writing to the old
// history table name.
func TestTableRenameKeepsTheRecordedVersions(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	cols := []schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text"}}
	files := evolve(ctx, t, db, nil, notesHistorySchema(t, cols...), "init", 1, migrate.DiffOptions{})
	if _, err := momo.Exec(ctx, db, "INSERT INTO notes (id, title) VALUES (1, 'before')"); err != nil {
		t.Fatal(err)
	}
	tbl := schema.Table{Name: "memos", Columns: cols, PrimaryKey: []string{"id"}}
	h, err := schema.HistoryFor(&tbl)
	if err != nil {
		t.Fatal(err)
	}
	renamed := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{tbl, h.Table}, Functions: []schema.Function{h.Function}, Triggers: []schema.Trigger{h.Trigger}}
	evolve(ctx, t, db, files, renamed, "rename_notes", 2, migrate.DiffOptions{RenameTables: map[string]string{"notes": "memos"}})

	if _, err := momo.Exec(ctx, db, "UPDATE memos SET title = 'after' WHERE id = 1"); err != nil {
		t.Fatalf("a write after the rename: %v", err)
	}
	got, err := momo.Scalars[string](ctx, db, "SELECT title FROM memos_history WHERE id = 1 ORDER BY history_id")
	if err != nil || len(got) != 2 || got[0] != "before" || got[1] != "after" {
		t.Fatalf("the versions on memos_history = %v (%v), want before then after", got, err)
	}
}
