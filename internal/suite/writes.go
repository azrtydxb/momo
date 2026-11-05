// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// delNote is a model without a soft-delete column (table del_notes).
type delNote struct {
	ID    int64
	Title string
}

var delNoteMeta = momo.Meta[delNote]{
	Table:      "del_notes",
	Columns:    []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "title"}},
	Ptr:        func(m *delNote, i int) any { return [...]any{&m.ID, &m.Title}[i] },
	Value:      func(m *delNote, i int) any { return [...]any{m.ID, m.Title}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (*delNote) MomoMeta() *momo.Meta[delNote] { return &delNoteMeta }

// sdUnix is a model whose soft-delete marker is a nullable Unix integer (table docs_unix).
type sdUnix struct {
	ID          int64
	TenantID    int64
	Title       string
	DeletedUnix *int64
}

var sdUnixTitle = momo.NewColumn[string]("docs_unix", "title", 0)

var sdUnixMeta = momo.Meta[sdUnix]{
	Table: "docs_unix",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "tenant_id"},
		{Name: "title"},
		{Name: "deleted_unix", Flags: momo.FlagSoftDelete | momo.FlagNullable},
	},
	Ptr:            func(m *sdUnix, i int) any { return [...]any{&m.ID, &m.TenantID, &m.Title, &m.DeletedUnix}[i] },
	Value:          func(m *sdUnix, i int) any { return [...]any{m.ID, m.TenantID, m.Title, m.DeletedUnix}[i] },
	Identity:       0,
	Version:        -1,
	SoftDelete:     3,
	SoftDeleteKind: momo.SoftDeleteUnix,
	CreatedAt:      -1,
	UpdatedAt:      -1,
	PK:             []int{0},
}

func (*sdUnix) MomoMeta() *momo.Meta[sdUnix] { return &sdUnixMeta }

// sdBool is a model whose soft-delete marker is a boolean (table docs_bool).
type sdBool struct {
	ID       int64
	TenantID int64
	Title    string
	Deleted  bool
}

var sdBoolTitle = momo.NewColumn[string]("docs_bool", "title", 0)

var sdBoolMeta = momo.Meta[sdBool]{
	Table: "docs_bool",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "tenant_id"},
		{Name: "title"},
		{Name: "deleted", Flags: momo.FlagSoftDelete},
	},
	Ptr:            func(m *sdBool, i int) any { return [...]any{&m.ID, &m.TenantID, &m.Title, &m.Deleted}[i] },
	Value:          func(m *sdBool, i int) any { return [...]any{m.ID, m.TenantID, m.Title, m.Deleted}[i] },
	Identity:       0,
	Version:        -1,
	SoftDelete:     3,
	SoftDeleteKind: momo.SoftDeleteBool,
	CreatedAt:      -1,
	UpdatedAt:      -1,
	PK:             []int{0},
}

func (*sdBool) MomoMeta() *momo.Meta[sdBool] { return &sdBoolMeta }

// execAll runs each statement, failing t on the first error.
func execAll(t *testing.T, db *momo.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := momo.Exec(context.Background(), db, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// dropOnCleanup drops the named tables when t ends.
func dropOnCleanup(t *testing.T, db *momo.DB, tables ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, tbl := range tables {
			if _, err := momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+tbl); err != nil {
				t.Errorf("drop %s: %v", tbl, err)
			}
		}
	})
}

// createDelNotes recreates del_notes with two rows.
func createDelNotes(t *testing.T, db *momo.DB) {
	t.Helper()
	execAll(t, db,
		"DROP TABLE IF EXISTS del_notes",
		"CREATE TABLE del_notes (id integer primary key, title text not null)",
		"INSERT INTO del_notes (id, title) VALUES (1, 'd1'), (2, 'd2')")
	dropOnCleanup(t, db, "del_notes")
}

// InsertModelOmitAndConflict asserts a model insert with an omitted column and
// an INSERT ... ON CONFLICT DO NOTHING that still inserts a new row.
func InsertModelOmitAndConflict(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	doc := Document{TenantID: 3, Title: "m", Secret: "s"}
	if _, err := momo.Insert[Document](db).Model(&doc).Omit(DocBody).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := momo.Insert[Document](db).
		Values(DocTenantID.Set(3), DocTitle.Set("m")).
		OnConflict(DocID).DoNothing().Exec(ctx)
	if err != nil || res.RowsAffected != 1 {
		t.Fatalf("insert on conflict do nothing with new row: %+v %v", res, err)
	}
	got, err := momo.Select[Document](db).Where(DocTitle.Eq("m")).Count(ctx)
	if err != nil || got != 2 {
		t.Fatalf("count = %d, %v", got, err)
	}
}

// MutationGuardrails asserts that an unfiltered UPDATE or DELETE is refused
// until AllRows is stated.
func MutationGuardrails(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "g"})
	if _, err := momo.Update[Document](db).Set(DocTitle.Set("x")).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("unfiltered update err = %v", err)
	}
	// documents is a soft-delete model, which Delete refuses outright, so the
	// unfiltered-DELETE guard is asserted on a model without one.
	createDelNotes(t, db)
	if _, err := momo.Delete[delNote](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("unfiltered delete err = %v", err)
	}
	res, err := momo.Update[Document](db).Set(DocTitle.Set("all")).AllRows().Exec(ctx)
	if err != nil || res.RowsAffected != 1 {
		t.Fatalf("AllRows update: %+v %v", res, err)
	}
}

// OptimisticLock asserts that a stale model update fails with ErrOptimisticLock
// and an update of a missing row with ErrNotFound.
func OptimisticLock(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "v"})
	doc, err := momo.Select[Document](db).First(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stale := doc
	doc.Title = "first"
	if _, err := momo.Update[Document](db).Model(&doc).Exec(ctx); err != nil {
		t.Fatalf("fresh update: %v", err)
	}
	stale.Title = "second"
	if _, err := momo.Update[Document](db).Model(&stale).Exec(ctx); !errors.Is(err, momo.ErrOptimisticLock) {
		t.Fatalf("stale update err = %v, want ErrOptimisticLock", err)
	}
	gone := doc
	gone.ID = 9999
	if _, err := momo.Update[Document](db).Model(&gone).Exec(ctx); !errors.Is(err, momo.ErrNotFound) {
		t.Fatalf("missing row err = %v, want ErrNotFound", err)
	}
}

// DeleteByModelAndGuard asserts delete by model key, the unfiltered guard (an
// empty And is not a filter), a nil model, and AllRows.
func DeleteByModelAndGuard(t *testing.T, db *momo.DB, _ Caps) {
	createDelNotes(t, db)
	ctx := context.Background()
	if res, err := momo.Delete[delNote](db).Model(&delNote{ID: 1}).Exec(ctx); err != nil || res.RowsAffected != 1 {
		t.Fatalf("delete by model: %+v %v", res, err)
	}
	if _, err := momo.Delete[delNote](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("unfiltered delete err = %v", err)
	}
	if _, err := momo.Delete[delNote](db).Where(momo.And()).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("Where(And()) delete err = %v", err)
	}
	if _, err := momo.Delete[delNote](db).Model(nil).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Model(nil) err = %v", err)
	}
	res, err := momo.Delete[delNote](db).AllRows().Exec(ctx)
	if err != nil || res.RowsAffected != 1 {
		t.Fatalf("AllRows delete: %+v %v", res, err)
	}
}

// SoftDelete asserts that SoftDelete marks a row once, and that default, with-
// deleted and only-deleted reads see the right rows.
func SoftDelete(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "keep"}, [2]any{1, "gone"})
	res, err := momo.SoftDelete[Document](db).Where(DocTitle.Eq("gone")).Exec(ctx)
	if err != nil || res.RowsAffected != 1 {
		t.Fatalf("SoftDelete: %+v %v", res, err)
	}
	if res, err := momo.SoftDelete[Document](db).Where(DocTitle.Eq("gone")).Exec(ctx); err != nil || res.RowsAffected != 0 {
		t.Fatalf("repeat SoftDelete: %+v %v", res, err)
	}
	visible, err := momo.Select[Document](db).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := momo.Select[Document](db).WithDeleted().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	only, err := momo.Select[Document](db).OnlyDeleted().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || len(all) != 2 || len(only) != 1 || only[0].Title != "gone" || only[0].DeletedAt == nil {
		t.Fatalf("visible=%d all=%d only=%d", len(visible), len(all), len(only))
	}
}

// SoftDeleteVariants asserts soft delete, re-creating a live key under a
// partial unique index, and ForceDelete for the timestamp, Unix and boolean
// markers.
func SoftDeleteVariants(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	id := "integer primary key"
	if caps.Dialect == dialect.Postgres {
		id = "bigint generated always as identity primary key"
	}
	execAll(t, db,
		"DROP TABLE IF EXISTS docs_unix",
		"DROP TABLE IF EXISTS docs_bool",
		"CREATE TABLE docs_unix (id "+id+", tenant_id integer not null, title text not null, deleted_unix bigint null)",
		"CREATE TABLE docs_bool (id "+id+", tenant_id integer not null, title text not null, deleted boolean not null default false)",
		"CREATE UNIQUE INDEX docs_unix_title ON docs_unix (tenant_id, title) WHERE deleted_unix IS NULL",
		"CREATE UNIQUE INDEX docs_bool_title ON docs_bool (tenant_id, title) WHERE deleted = FALSE",
		"CREATE UNIQUE INDEX documents_title_live ON documents (tenant_id, title) WHERE deleted_at IS NULL",
		"INSERT INTO documents (tenant_id, title) VALUES (5, 'u')",
		"INSERT INTO docs_unix (tenant_id, title) VALUES (5, 'u')",
		"INSERT INTO docs_bool (tenant_id, title) VALUES (5, 'u')")
	dropOnCleanup(t, db, "docs_unix", "docs_bool")

	t.Run("timestamp", func(t *testing.T) { softDeleteVariant[Document](t, db, DocTitle, "documents") })
	t.Run("unix", func(t *testing.T) { softDeleteVariant[sdUnix](t, db, sdUnixTitle, "docs_unix") })
	t.Run("bool", func(t *testing.T) { softDeleteVariant[sdBool](t, db, sdBoolTitle, "docs_bool") })
}

func softDeleteVariant[T any, PT momo.Modeler[T]](t *testing.T, db *momo.DB, title momo.Column[string], table string) {
	t.Helper()
	ctx := context.Background()
	if res, err := momo.SoftDelete[T, PT](db).Where(title.Eq("u")).Exec(ctx); err != nil || res.RowsAffected != 1 {
		t.Fatalf("SoftDelete: %+v %v", res, err)
	}
	if n, err := momo.Select[T, PT](db).Where(title.Eq("u")).Count(ctx); err != nil || n != 0 {
		t.Fatalf("visible after soft delete = %d, %v, want 0", n, err)
	}
	if n, err := momo.Select[T, PT](db).OnlyDeleted().Where(title.Eq("u")).Count(ctx); err != nil || n != 1 {
		t.Fatalf("OnlyDeleted = %d, %v, want 1", n, err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+table+" (tenant_id, title) VALUES (5, 'u')"); err != nil {
		t.Fatalf("re-create soft-deleted key: %v", err)
	}
	if n, err := momo.Select[T, PT](db).Where(title.Eq("u")).Count(ctx); err != nil || n != 1 {
		t.Fatalf("visible after re-create = %d, %v, want 1", n, err)
	}
	if n, err := momo.Select[T, PT](db).WithDeleted().Where(title.Eq("u")).Count(ctx); err != nil || n != 2 {
		t.Fatalf("WithDeleted = %d, %v, want 2", n, err)
	}
	if res, err := momo.ForceDelete[T, PT](db).Where(title.Eq("u")).Exec(ctx); err != nil || res.RowsAffected != 2 {
		t.Fatalf("ForceDelete: %+v %v, want 2 rows", res, err)
	}
	if n, err := momo.Select[T, PT](db).WithDeleted().Where(title.Eq("u")).Count(ctx); err != nil || n != 0 {
		t.Fatalf("ForceDelete left %d rows, %v", n, err)
	}
}

// UpsertUpdatesAllColumns asserts that an upsert by primary key updates every
// column and never duplicates the row.
func UpsertUpdatesAllColumns(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	doc := Document{ID: 77, TenantID: 1, Title: "first"}
	if _, err := momo.Upsert[Document](db).Model(&doc).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	doc.Title = "second"
	doc.TenantID = 9
	if _, err := momo.Upsert[Document](db).Model(&doc).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := momo.Select[Document](db).Where(DocID.Eq(77)).One(ctx)
	if err != nil || got.Title != "second" || got.TenantID != 9 {
		t.Fatalf("upsert did not update all columns: %+v %v", got, err)
	}
	if n, err := momo.Select[Document](db).Count(ctx); err != nil || n != 1 {
		t.Fatalf("rows = %d, %v; want 1", n, err)
	}
}

// AutoTimestamps asserts that insert sets created_at and updated_at from the
// clock, a matching update advances updated_at only, an update that matches no
// row changes nothing, and an explicit value wins on insert.
func AutoTimestamps(t *testing.T, db *momo.DB, caps Caps) {
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	later := fixed.Add(time.Hour)
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	db1 := momo.New(db.Driver(), momo.WithClock(func() time.Time { return fixed }))
	db2 := momo.New(db.Driver(), momo.WithClock(func() time.Time { return later }))
	explicit := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

	if _, err := momo.Insert[Document](db1).Values(DocTenantID.Set(1), DocTitle.Set("ts")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	d, err := momo.Select[Document](db1).Where(DocTitle.Eq("ts")).One(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !d.CreatedAt.Equal(fixed) || !d.UpdatedAt.Equal(fixed) {
		t.Fatalf("insert timestamps = %v %v, want %v", d.CreatedAt, d.UpdatedAt, fixed)
	}

	res, err := momo.Update[Document](db2).Set(DocTitle.Set("ts2")).Where(DocTitle.Eq("nomatch")).Exec(ctx)
	if err != nil || res.RowsAffected != 0 {
		t.Fatalf("no-match update: %+v %v", res, err)
	}
	d2, err := momo.Select[Document](db1).Where(DocTitle.Eq("ts")).One(ctx)
	if err != nil || !d2.UpdatedAt.Equal(fixed) {
		t.Fatalf("no-match update changed updated_at to %v (%v)", d2.UpdatedAt, err)
	}

	if _, err := momo.Update[Document](db2).Set(DocTitle.Set("ts2")).Where(DocTitle.Eq("ts")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	d3, err := momo.Select[Document](db1).Where(DocTitle.Eq("ts2")).One(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !d3.UpdatedAt.Equal(later) || !d3.CreatedAt.Equal(fixed) {
		t.Fatalf("update timestamps = created %v updated %v", d3.CreatedAt, d3.UpdatedAt)
	}

	if _, err := momo.Insert[Document](db1).Values(DocTenantID.Set(1), DocTitle.Set("ex"), DocCreatedAt.Set(explicit)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	e, err := momo.Select[Document](db1).Where(DocTitle.Eq("ex")).One(ctx)
	if err != nil || !e.CreatedAt.Equal(explicit) || !e.UpdatedAt.Equal(fixed) {
		t.Fatalf("explicit insert = created %v updated %v (%v)", e.CreatedAt, e.UpdatedAt, err)
	}
}
