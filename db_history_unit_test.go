// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// unitNote is a model with an audit history, shaped like generated code.
type unitNote struct {
	ID    int64
	Title string
	Body  *string
}

var unitNoteMeta = momo.Meta[unitNote]{
	Table: "notes",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "title"},
		{Name: "body", Flags: momo.FlagNullable},
	},
	Ptr:      func(m *unitNote, i int) any { return [...]any{&m.ID, &m.Title, &m.Body}[i] },
	Value:    func(m *unitNote, i int) any { return [...]any{m.ID, m.Title, m.Body}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*unitNote) MomoMeta() *momo.Meta[unitNote] { return &unitNoteMeta }

// unitNoteView is a view model: it has no history to read.
type unitNoteView struct {
	ID int64
}

var unitNoteViewMeta = momo.Meta[unitNoteView]{
	Table:    "note_view",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}},
	Ptr:      func(m *unitNoteView, i int) any { return &m.ID },
	Value:    func(m *unitNoteView, i int) any { return m.ID },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:   []int{0},
	View: true,
}

func (*unitNoteView) MomoMeta() *momo.Meta[unitNoteView] { return &unitNoteViewMeta }

const historyColumns = `SELECT "id", "title", "body", "history_id", "history_operation", "history_changed_at", "history_changed_by" FROM "notes_history" AS "notes" WHERE "id" = $1`

// Catches: a history query that reads the model table instead of the history
// table, orders the versions the wrong way, or binds the key as text.
func TestHistoryOfSQL(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	if _, err := momo.HistoryOf[unitNote](context.Background(), db, int64(7)); err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one statement", calls)
	}
	want := historyColumns + ` ORDER BY "history_id"`
	if calls[0].SQL != want {
		t.Errorf("SQL\n got %s\nwant %s", calls[0].SQL, want)
	}
	if len(calls[0].Args) != 1 || calls[0].Args[0] != int64(7) {
		t.Errorf("args = %v, want [7]", calls[0].Args)
	}
}

// Catches: an as-of query that picks the earliest version instead of the
// latest one at or before the time, or that forgets to bound it.
func TestHistoryAsOfSQL(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	_, err := momo.HistoryAsOf[unitNote](context.Background(), db, at, int64(7))
	if !errors.Is(err, momo.ErrNotFound) {
		t.Fatalf("no recorded version: %v, want ErrNotFound", err)
	}
	calls := drv.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one statement", calls)
	}
	want := historyColumns + ` AND "history_changed_at" <= $2 ORDER BY "history_changed_at" DESC, "history_id" DESC LIMIT 1`
	if calls[0].SQL != want {
		t.Errorf("SQL\n got %s\nwant %s", calls[0].SQL, want)
	}
	if len(calls[0].Args) != 2 || calls[0].Args[0] != int64(7) || calls[0].Args[1] != at {
		t.Errorf("args = %v, want [7 %v]", calls[0].Args, at)
	}
}

// Catches: a history read that ignores the tenant of the context (it would
// return another tenant's audit rows), or that widens when the tenant is
// missing or has the wrong type instead of failing before any statement runs.
func TestHistoryReadsApplyTheTenantFilter(t *testing.T) {
	tenant := momo.NewColumn[int64]("notes", "tenant_id", 0)
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	ctx := momo.WithTenant(context.Background(), int64(5))
	if _, err := momo.HistoryOf[unitNote](ctx, db, int64(7), momo.TenantFilter(ctx, tenant)); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if _, err := momo.HistoryAsOf[unitNote](ctx, db, at, int64(7), momo.TenantFilter(ctx, tenant)); !errors.Is(err, momo.ErrNotFound) {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v, want two statements", calls)
	}
	wantOf := historyColumns + ` AND "notes"."tenant_id" = $2 ORDER BY "history_id"`
	if calls[0].SQL != wantOf || len(calls[0].Args) != 2 || calls[0].Args[1] != int64(5) {
		t.Errorf("HistoryOf: SQL %s args %v, want %s with the tenant bound", calls[0].SQL, calls[0].Args, wantOf)
	}
	wantAt := historyColumns + ` AND "notes"."tenant_id" = $2 AND "history_changed_at" <= $3 ORDER BY "history_changed_at" DESC, "history_id" DESC LIMIT 1`
	if calls[1].SQL != wantAt {
		t.Errorf("HistoryAsOf: SQL %s, want %s", calls[1].SQL, wantAt)
	}

	for name, bad := range map[string]context.Context{
		"no tenant":         context.Background(),
		"a mistyped tenant": momo.WithTenant(context.Background(), "five"),
	} {
		before := len(drv.Calls())
		if _, err := momo.HistoryOf[unitNote](bad, db, int64(7), momo.TenantFilter(bad, tenant)); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("HistoryOf with %s: %v, want ErrValidation", name, err)
		}
		if _, err := momo.HistoryAsOf[unitNote](bad, db, at, int64(7), momo.TenantFilter(bad, tenant)); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("HistoryAsOf with %s: %v, want ErrValidation", name, err)
		}
		if n := len(drv.Calls()); n != before {
			t.Errorf("%s: %d statements ran for a refused read", name, n-before)
		}
	}
}

// Catches: a row scanned into the wrong fields, a NULL actor turned into an
// empty string, and a DELETE reported as the state at a time.
func TestHistoryRowsScan(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	t0 := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	drv.SetRows([][]any{
		{int64(7), "first", nil, int64(1), "INSERT", t0, nil},
		{int64(7), "second", "body", int64(2), "UPDATE", t0.Add(time.Minute), "alice"},
	})
	got, err := momo.HistoryOf[unitNote](ctx, db, int64(7))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("versions = %+v, want 2", got)
	}
	if g := got[0]; g.ID != 1 || g.Operation != momo.HistoryInsert || !g.ChangedAt.Equal(t0) || g.ChangedBy != nil || g.Row.Title != "first" || g.Row.Body != nil || g.Row.ID != 7 {
		t.Errorf("first version = %+v", g)
	}
	if g := got[1]; g.ID != 2 || g.Operation != momo.HistoryUpdate || g.ChangedBy == nil || *g.ChangedBy != "alice" || g.Row.Body == nil || *g.Row.Body != "body" {
		t.Errorf("second version = %+v", g)
	}

	drv.SetRows([][]any{{int64(7), "second", nil, int64(3), "UPDATE", t0, "bob"}})
	at, err := momo.HistoryAsOf[unitNote](ctx, db, t0, int64(7))
	if err != nil || at.Row.Title != "second" || at.ID != 3 {
		t.Fatalf("as of a time = %+v, %v", at, err)
	}
	drv.SetRows([][]any{{int64(7), "second", nil, int64(4), "DELETE", t0, "bob"}})
	if _, err := momo.HistoryAsOf[unitNote](ctx, db, t0, int64(7)); !errors.Is(err, momo.ErrNotFound) {
		t.Errorf("the latest version is a delete: %v, want ErrNotFound", err)
	}
}

// Catches: a dialect without transaction-local settings being sent history
// SQL, and a malformed call reaching the database instead of being refused
// before any statement runs.
func TestHistoryRefusals(t *testing.T) {
	ctx := context.Background()
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		drv := fakedriver.New(d)
		db := momo.New(drv)
		if _, err := momo.HistoryOf[unitNote](ctx, db, int64(1)); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: HistoryOf: %v, want ErrUnsupported", d, err)
		}
		if _, err := momo.HistoryAsOf[unitNote](ctx, db, time.Now(), int64(1)); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: HistoryAsOf: %v, want ErrUnsupported", d, err)
		}
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error { return momo.SetActor(ctx, tx, "alice") })
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: SetActor: %v, want ErrUnsupported", d, err)
		}
		if n := len(drv.Calls()); n != 0 {
			t.Errorf("%s: %d statements ran for a refused call", d, n)
		}
	}

	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	if _, err := momo.HistoryOf[unitNote](ctx, db); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("no key values: %v, want ErrValidation", err)
	}
	if _, err := momo.HistoryOf[unitNote](ctx, db, int64(1), int64(2)); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("two key values for a one-column key: %v, want ErrValidation", err)
	}
	if _, err := momo.HistoryOf[unitNoteView](ctx, db, int64(1)); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("a view model: %v, want ErrValidation", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Errorf("%d statements ran for a refused call", n)
	}
}

// Catches: an actor set with a session-level SET (it would reach the next user
// of a pooled connection) or spliced into the SQL text instead of bound.
func TestSetActorIsTransactionLocalAndBound(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	hostile := "x'); DROP TABLE notes; --"
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error { return momo.SetActor(ctx, tx, hostile) })
	if err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want one statement", calls)
	}
	if calls[0].SQL != "SELECT set_config($1, $2, true)" {
		t.Errorf("SQL = %s, want a transaction-local set_config", calls[0].SQL)
	}
	if len(calls[0].Args) != 2 || calls[0].Args[0] != "momo.actor" || calls[0].Args[1] != hostile {
		t.Errorf("args = %v, want the setting name and the actor, bound", calls[0].Args)
	}
}
