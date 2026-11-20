// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// auditNote is a model with an audit history. Its metadata is written by hand,
// as the generator would write it.
type auditNote struct {
	ID    int64
	Title string
	Body  *string
}

var auditNoteMeta = momo.Meta[auditNote]{
	Table: "audit_notes",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "title"},
		{Name: "body", Flags: momo.FlagNullable},
	},
	Ptr:      func(m *auditNote, i int) any { return [...]any{&m.ID, &m.Title, &m.Body}[i] },
	Value:    func(m *auditNote, i int) any { return [...]any{m.ID, m.Title, m.Body}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*auditNote) MomoMeta() *momo.Meta[auditNote] { return &auditNoteMeta }

// auditNoteSchema is the schema the generator builds for auditNote with
// Config.History: the table, and the history table, function and trigger of
// schema.HistoryFor.
func auditNoteSchema(t *testing.T) *schema.Schema {
	t.Helper()
	tbl := schema.Table{
		Name:       "audit_notes",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text"}, {Name: "body", Type: "text", Nullable: true}},
		PrimaryKey: []string{"id"},
	}
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

// Catches: a history that misses a change, records the wrong actor, leaks an
// actor from one transaction into the next, or answers "as of a time" with the
// wrong version or with a row that did not exist yet or was already deleted.
func TestHistoryRecordsEveryChangeWithItsActor(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := auditNoteSchema(t)
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, desired, "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	dbNow := func() time.Time {
		t.Helper()
		ts, err := momo.QueryScalars[time.Time](ctx, db, "SELECT clock_timestamp()")
		if err != nil || len(ts) != 1 {
			t.Fatalf("clock: %v %v", ts, err)
		}
		return ts[0]
	}
	as := func(actor, sql string, args ...any) {
		t.Helper()
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := momo.SetActor(ctx, tx, actor); err != nil {
				return err
			}
			_, err := momo.Exec(ctx, tx, sql, args...)
			return err
		})
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	beforeAll := dbNow()
	if _, err := momo.Exec(ctx, db, "INSERT INTO audit_notes (id, title) VALUES (1, 'v1')"); err != nil {
		t.Fatal(err)
	}
	as("alice", "UPDATE audit_notes SET title = 'v2' WHERE id = 1")
	mid := dbNow()
	as("bob", "UPDATE audit_notes SET title = 'v3', body = 'text' WHERE id = 1")
	as("carol", "DELETE FROM audit_notes WHERE id = 1")
	afterAll := dbNow()
	// A later change in a transaction that never set an actor must not see
	// the actor of an earlier one on the same pooled connection.
	if _, err := momo.Exec(ctx, db, "INSERT INTO audit_notes (id, title) VALUES (2, 'other')"); err != nil {
		t.Fatal(err)
	}

	got, err := momo.HistoryOf[auditNote](ctx, db, int64(1))
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		op, title string
		by        string // empty: NULL
	}
	wants := []want{{"INSERT", "v1", ""}, {"UPDATE", "v2", "alice"}, {"UPDATE", "v3", "bob"}, {"DELETE", "v3", "carol"}}
	if len(got) != len(wants) {
		t.Fatalf("HistoryOf returned %d versions, want %d: %+v", len(got), len(wants), got)
	}
	for i, w := range wants {
		g := got[i]
		by := ""
		if g.ChangedBy != nil {
			by = *g.ChangedBy
		}
		if g.Operation != w.op || g.Row.Title != w.title || by != w.by || g.Row.ID != 1 {
			t.Errorf("version %d = %s %q by %q (row %+v), want %s %q by %q", i, g.Operation, g.Row.Title, by, g.Row, w.op, w.title, w.by)
		}
		if i > 0 && (g.ID <= got[i-1].ID || g.ChangedAt.Before(got[i-1].ChangedAt)) {
			t.Errorf("version %d is not after version %d: %+v", i, i-1, got)
		}
	}
	if got[2].Row.Body == nil || *got[2].Row.Body != "text" {
		t.Errorf("the body of version 2 = %v, want text", got[2].Row.Body)
	}
	other, err := momo.HistoryOf[auditNote](ctx, db, int64(2))
	if err != nil || len(other) != 1 || other[0].ChangedBy != nil {
		t.Fatalf("a change made with no actor set recorded %+v (%v), want one version with a NULL actor", other, err)
	}
	none, err := momo.HistoryOf[auditNote](ctx, db, int64(99))
	if err != nil || len(none) != 0 {
		t.Fatalf("a row with no history gave %+v, %v, want an empty result", none, err)
	}

	at, err := momo.HistoryAsOf[auditNote](ctx, db, mid, int64(1))
	if err != nil || at.Row.Title != "v2" || at.ChangedBy == nil || *at.ChangedBy != "alice" {
		t.Fatalf("state at the middle = %+v, %v, want v2 by alice", at, err)
	}
	for name, when := range map[string]time.Time{"before the insert": beforeAll, "after the delete": afterAll} {
		if _, err := momo.HistoryAsOf[auditNote](ctx, db, when, int64(1)); !errors.Is(err, momo.ErrNotFound) {
			t.Errorf("state %s: %v, want ErrNotFound", name, err)
		}
	}
	if _, err := momo.HistoryOf[auditNote](ctx, db, int64(1), int64(2)); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("two key values for a one-column key: %v, want ErrValidation", err)
	}
}

// applyAuditNotes creates the audit_notes table and its history objects.
func applyAuditNotes(ctx context.Context, t *testing.T, db *momo.DB) {
	t.Helper()
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, auditNoteSchema(t), "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
}

func execAll(ctx context.Context, t *testing.T, db *momo.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := momo.Exec(ctx, db, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// Catches: an UPDATE of the primary key that leaves the old key's timeline
// open (the old key would still read as existing after the row moved away), or
// a new key whose history does not start with its first row.
func TestPrimaryKeyChangeClosesTheOldKeysTimeline(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	applyAuditNotes(ctx, t, db)
	execAll(ctx, t,
		db, "INSERT INTO audit_notes (id, title) VALUES (1, 'moving')",
		"UPDATE audit_notes SET id = 2 WHERE id = 1")
	after, err := momo.QueryScalars[time.Time](ctx, db, "SELECT clock_timestamp()")
	if err != nil || len(after) != 1 {
		t.Fatal(err)
	}

	if _, err := momo.HistoryAsOf[auditNote](ctx, db, after[0], int64(1)); !errors.Is(err, momo.ErrNotFound) {
		t.Errorf("the old key after the move: %v, want ErrNotFound", err)
	}
	old, err := momo.HistoryOf[auditNote](ctx, db, int64(1))
	if err != nil || len(old) != 2 || old[0].Operation != momo.HistoryInsert || old[1].Operation != momo.HistoryDelete {
		t.Fatalf("history of the old key = %+v (%v), want an INSERT then a DELETE", old, err)
	}
	moved, err := momo.HistoryOf[auditNote](ctx, db, int64(2))
	if err != nil || len(moved) != 1 || moved[0].Operation != momo.HistoryUpdate || moved[0].Row.ID != 2 || moved[0].Row.Title != "moving" {
		t.Fatalf("history of the new key = %+v (%v), want one row, the new version", moved, err)
	}
	if now, err := momo.HistoryAsOf[auditNote](ctx, db, after[0], int64(2)); err != nil || now.Row.ID != 2 {
		t.Errorf("the new key after the move: %+v, %v, want the row", now, err)
	}
}

// Catches: an UPDATE that changes nothing writing a history row, which would
// bury the real changes in noise, and a real change after it being skipped.
func TestNoOpUpdateRecordsNothing(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	applyAuditNotes(ctx, t, db)
	execAll(ctx, t, db,
		"INSERT INTO audit_notes (id, title) VALUES (1, 'same')",
		"UPDATE audit_notes SET title = 'same' WHERE id = 1",
		"UPDATE audit_notes SET title = title, body = NULL WHERE id = 1")
	got, err := momo.HistoryOf[auditNote](ctx, db, int64(1))
	if err != nil || len(got) != 1 {
		t.Fatalf("history after two no-op updates = %+v (%v), want only the insert", got, err)
	}
	execAll(ctx, t, db, "UPDATE audit_notes SET title = 'changed' WHERE id = 1")
	if got, err = momo.HistoryOf[auditNote](ctx, db, int64(1)); err != nil || len(got) != 2 || got[1].Row.Title != "changed" {
		t.Fatalf("history after a real change = %+v (%v), want two versions", got, err)
	}
}

// Catches: the actor of one transaction surviving on the connection into the
// next write, where it would be recorded as that writer's name. The pool has
// one connection, so the next statement runs on the connection the first
// transaction used.
func TestActorDoesNotOutliveItsTransaction(t *testing.T) {
	ctx := context.Background()
	setup := testopen.Postgres(t)
	applyAuditNotes(ctx, t, setup)

	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db := momo.New(pgxdriver.New(pool))

	err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := momo.SetActor(ctx, tx, "alice"); err != nil {
			return err
		}
		_, err := momo.Exec(ctx, tx, "INSERT INTO audit_notes (id, title) VALUES (1, 'by alice')")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	left, err := momo.QueryScalars[string](ctx, db, "SELECT coalesce(current_setting('momo.actor', true), '')")
	if err != nil || len(left) != 1 || left[0] != "" {
		t.Fatalf("momo.actor after the commit = %q (%v), want empty", left, err)
	}
	execAll(ctx, t, db, "INSERT INTO audit_notes (id, title) VALUES (2, 'by nobody')")
	if n := pool.Stat().TotalConns(); n != 1 {
		t.Fatalf("the pool holds %d connections, want the one every statement shared", n)
	}
	got, err := momo.HistoryOf[auditNote](ctx, db, int64(2))
	if err != nil || len(got) != 1 || got[0].ChangedBy != nil {
		t.Fatalf("the later write recorded %+v (%v), want a NULL actor", got, err)
	}
}

// Catches: a soft delete read as a removal of the row. It is an UPDATE that sets
// the deleted marker, so the history records an UPDATE and the row still exists
// as of that time; only a hard DELETE ends it. The body column stands in for
// the model's deleted_at marker: any UPDATE is recorded the same way.
func TestSoftDeleteIsRecordedAsAnUpdate(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	applyAuditNotes(ctx, t, db)
	execAll(ctx, t, db,
		"INSERT INTO audit_notes (id, title) VALUES (1, 'live')",
		"UPDATE audit_notes SET body = 'deleted_at set' WHERE id = 1")
	got, err := momo.HistoryOf[auditNote](ctx, db, int64(1))
	if err != nil || len(got) != 2 || got[1].Operation != momo.HistoryUpdate {
		t.Fatalf("history = %+v (%v), want an INSERT then an UPDATE", got, err)
	}
	now, err := momo.QueryScalars[time.Time](ctx, db, "SELECT clock_timestamp()")
	if err != nil || len(now) != 1 {
		t.Fatal(err)
	}
	if at, err := momo.HistoryAsOf[auditNote](ctx, db, now[0], int64(1)); err != nil || at.Operation != momo.HistoryUpdate {
		t.Fatalf("state after the soft delete = %+v, %v, want the row as an UPDATE", at, err)
	}
}

// Catches: a dialect without transaction-local settings being sent the
// PostgreSQL history query or set_config, instead of failing with
// ErrUnsupported before any SQL runs.
func TestHistoryIsRefusedOnSQLite(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	if _, err := momo.HistoryOf[auditNote](ctx, db, int64(1)); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("HistoryOf on SQLite: %v, want ErrUnsupported", err)
	}
	if _, err := momo.HistoryAsOf[auditNote](ctx, db, time.Now(), int64(1)); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("HistoryAsOf on SQLite: %v, want ErrUnsupported", err)
	}
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error { return momo.SetActor(ctx, tx, "alice") })
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("SetActor on SQLite: %v, want ErrUnsupported", err)
	}
}
