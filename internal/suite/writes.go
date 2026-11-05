// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"database/sql"
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
	switch caps.Dialect {
	case dialect.Postgres:
		id = "bigint generated always as identity primary key"
	case dialect.MySQL:
		id = "bigint auto_increment primary key"
	}
	execAll(t, db,
		"DROP TABLE IF EXISTS docs_unix",
		"DROP TABLE IF EXISTS docs_bool",
		"CREATE TABLE docs_unix (id "+id+", tenant_id integer not null, title text not null, deleted_unix bigint null)",
		"CREATE TABLE docs_bool (id "+id+", tenant_id integer not null, title text not null, deleted boolean not null default false)")
	if caps.PartialIndexes {
		// The partial unique indexes make the re-create step prove the key is
		// free again once the old row is soft-deleted. Without partial
		// indexes the step still runs, with nothing to conflict on.
		execAll(t, db,
			"CREATE UNIQUE INDEX docs_unix_title ON docs_unix (tenant_id, title) WHERE deleted_unix IS NULL",
			"CREATE UNIQUE INDEX docs_bool_title ON docs_bool (tenant_id, title) WHERE deleted = FALSE",
			"CREATE UNIQUE INDEX documents_title_live ON documents (tenant_id, title) WHERE deleted_at IS NULL")
	}
	execAll(t, db,
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

	// An upsert under a later clock advances updated_at and leaves created_at.
	first := time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)
	later := first.Add(2 * time.Hour)
	tdb1 := momo.New(db.Driver(), momo.WithClock(func() time.Time { return first }))
	tdb2 := momo.New(db.Driver(), momo.WithClock(func() time.Time { return later }))
	ts := Document{ID: 78, TenantID: 1, Title: "ts-first"}
	if _, err := momo.Upsert[Document](tdb1).Model(&ts).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	ts.Title = "ts-second"
	if _, err := momo.Upsert[Document](tdb2).Model(&ts).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	tg, err := momo.Select[Document](db).Where(DocID.Eq(78)).One(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !tg.UpdatedAt.Equal(later) || !tg.CreatedAt.Equal(first) || tg.Title != "ts-second" {
		t.Fatalf("upsert timestamps = created %v updated %v title %q, want created %v updated %v", tg.CreatedAt, tg.UpdatedAt, tg.Title, first, later)
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

// statusOf reads documents.status for one title. Every error is checked.
func statusOf(t *testing.T, db *momo.DB, title string) sql.NullString {
	t.Helper()
	vals, err := momo.Scalars[sql.NullString](context.Background(), db, "SELECT status FROM documents WHERE title = ?", title)
	if err != nil || len(vals) != 1 {
		t.Fatalf("status of %q: err=%v rows=%d", title, err, len(vals))
	}
	return vals[0]
}

// WriteStates runs the UPDATE write states: omitted, zero, NULL and DEFAULT.
func WriteStates(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	for _, title := range []string{"u-omit", "u-zero", "u-null", "u-default"} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title, status) VALUES (1, ?, 'x')", title); err != nil {
			t.Fatal(err)
		}
	}
	// omitted: status is not in SET, so 'x' stays
	if _, err := momo.Update[StatusDocument](db).Set(DocTitle.Set("u-omit")).Where(DocTitle.Eq("u-omit")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	// zero: the empty string is written
	if _, err := momo.Update[StatusDocument](db).Set(DocStatus.Set("")).Where(DocTitle.Eq("u-zero")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	// NULL
	if _, err := momo.Update[StatusDocument](db).Set(DocStatus.SetNull()).Where(DocTitle.Eq("u-null")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	// DEFAULT: PostgreSQL stores 'draft'; SQLite rejects it at build time
	_, err := momo.Update[StatusDocument](db).Set(DocStatus.SetDefault()).Where(DocTitle.Eq("u-default")).Exec(ctx)
	if !caps.DefaultInUpdate {
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("SET DEFAULT on %s: err = %v, want ErrUnsupported", caps.Dialect, err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, db, "u-omit"); !got.Valid || got.String != "x" {
		t.Fatalf("omitted kept %v, want 'x'", got)
	}
	if got := statusOf(t, db, "u-zero"); !got.Valid || got.String != "" {
		t.Fatalf("zero stored %v, want ''", got)
	}
	if got := statusOf(t, db, "u-null"); got.Valid {
		t.Fatalf("NULL stored %v, want NULL", got)
	}
	want := "x"
	if caps.DefaultInUpdate {
		want = "draft"
	}
	if got := statusOf(t, db, "u-default"); !got.Valid || got.String != want {
		t.Fatalf("DEFAULT stored %v, want %q", got, want)
	}
}

// WriteStatesInsert runs the single-row INSERT write states.
func WriteStatesInsert(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	if _, err := momo.Insert[StatusDocument](db).Values(DocTenantID.Set(1), DocTitle.Set("i-omit")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		title string
		a     momo.Assignment
	}{
		{"i-zero", DocStatus.Set("")},
		{"i-null", DocStatus.SetNull()},
		{"i-default", DocStatus.SetDefault()},
	} {
		if _, err := momo.Insert[StatusDocument](db).Values(DocTenantID.Set(1), DocTitle.Set(c.title), c.a).Exec(ctx); err != nil {
			t.Fatalf("insert %s: %v", c.title, err)
		}
	}
	want := map[string]sql.NullString{
		"i-omit":    {String: "draft", Valid: true},
		"i-zero":    {String: "", Valid: true},
		"i-null":    {},
		"i-default": {String: "draft", Valid: true},
	}
	for title, w := range want {
		if got := statusOf(t, db, title); got != w {
			t.Fatalf("%s stored %v, want %v", title, got, w)
		}
	}
}

// WriteStatesMultiRow runs the multi-row INSERT write states.
func WriteStatesMultiRow(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	uniform := momo.Insert[StatusDocument](db).ValuesRows(
		[]momo.Assignment{DocTenantID.Set(1), DocTitle.Set("m1"), DocStatus.SetDefault()},
		[]momo.Assignment{DocTenantID.Set(1), DocTitle.Set("m2"), DocStatus.SetDefault()},
	)
	if _, err := uniform.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"m1", "m2"} {
		if got := statusOf(t, db, title); !got.Valid || got.String != "draft" {
			t.Fatalf("%s = %v, want 'draft'", title, got)
		}
	}
	mixed := momo.Insert[StatusDocument](db).ValuesRows(
		[]momo.Assignment{DocTenantID.Set(1), DocTitle.Set("n1"), DocStatus.SetDefault()},
		[]momo.Assignment{DocTenantID.Set(1), DocTitle.Set("n2"), DocStatus.Set("y")},
	)
	_, err := mixed.Exec(ctx)
	if !caps.DefaultInValues {
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("mixed DEFAULT on %s: err = %v, want ErrUnsupported", caps.Dialect, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if n1 := statusOf(t, db, "n1"); !n1.Valid || n1.String != "draft" {
		t.Fatalf("n1 = %v, want 'draft'", n1)
	}
	if n2 := statusOf(t, db, "n2"); !n2.Valid || n2.String != "y" {
		t.Fatalf("n2 = %v, want 'y'", n2)
	}
}
