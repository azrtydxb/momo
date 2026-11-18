// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
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
		ts, err := momo.Scalars[time.Time](ctx, db, "SELECT clock_timestamp()")
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
