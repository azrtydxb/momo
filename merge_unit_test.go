// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

// versioned is a fake driver that reports a server version.
type versioned struct {
	*fakedriver.Driver
	version int
}

func (v versioned) ServerVersion(context.Context) (int, error) { return v.version, nil }

func newVersioned(version int) versioned {
	return versioned{Driver: fakedriver.New(dialect.Postgres), version: version}
}

func mergeInsertQuery(q momo.Querier) momo.MergeQuery[suite.Document] {
	src := momo.RawSubquery("SELECT 3 AS id, 1 AS tenant_id, 'merged' AS title")
	return momo.Merge[suite.Document](q, src).
		On(suite.DocID.EqExpr(momo.TypedSQL[int64]("src.id"))).
		WhenNotMatchedInsert([]momo.ColumnRef{suite.DocID, suite.DocTenantID, suite.DocTitle},
			momo.TypedSQL[int64]("src.id"), momo.TypedSQL[int64]("src.tenant_id"), momo.TypedSQL[string]("src.title"))
}

func hasAll(t *testing.T, sql string, frags ...string) {
	t.Helper()
	for _, f := range frags {
		if !strings.Contains(sql, f) {
			t.Errorf("SQL lacks %q:\n%s", f, sql)
		}
	}
}

// Catches: a wrong clause order, a dropped OVERRIDING SYSTEM VALUE for an
// identity column, a lost automatic timestamp and a missing version bump.
func TestMergeRendersMatchedAndNotMatched(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	c, err := mergeInsertQuery(db).WhenMatchedUpdate(suite.DocTitle.Set("zzval")).Returning(suite.DocID).SQL()
	if err != nil {
		t.Fatal(err)
	}
	hasAll(t, c.SQL,
		`MERGE INTO "documents" USING (SELECT 3 AS id, 1 AS tenant_id, 'merged' AS title) AS "src" ON "documents"."id" = src.id`,
		` WHEN MATCHED THEN UPDATE SET "title" = $`,
		`"version" = "documents"."version" + 1`,
		` WHEN NOT MATCHED THEN INSERT ("id", "tenant_id", "title", "created_at", "updated_at") OVERRIDING SYSTEM VALUE VALUES (src.id, src.tenant_id, src.title, $`,
		` RETURNING "id"`)
	if strings.Index(c.SQL, "WHEN MATCHED") > strings.Index(c.SQL, "WHEN NOT MATCHED") {
		t.Errorf("WHEN MATCHED must come first:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "zzval") {
		t.Errorf("a value was inlined:\n%s", c.SQL)
	}
}

// Catches: MERGE rendering on a dialect without it, or a server call before
// the dialect check.
func TestMergeUnsupportedOnSQLite(t *testing.T) {
	drv := fakedriver.New(dialect.SQLite)
	db := momo.New(drv)
	if _, err := mergeInsertQuery(db).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("SQL = %v, want ErrUnsupported", err)
	}
	if _, err := mergeInsertQuery(db).Exec(t.Context()); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Exec = %v, want ErrUnsupported", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("calls = %v, want none", drv.Calls())
	}
}

// Catches: an unguarded MERGE (no join condition, no action) or one that
// writes a column Update refuses.
func TestMergeValidation(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	src := momo.RawSubquery("SELECT 1 AS id")
	on := suite.DocID.EqExpr(momo.TypedSQL[int64]("src.id"))
	cols := []momo.ColumnRef{suite.DocTitle}
	cases := map[string]momo.MergeQuery[suite.Document]{
		"no on":              momo.Merge[suite.Document](db, src).WhenMatchedUpdate(suite.DocTitle.Set("x")),
		"no clause":          momo.Merge[suite.Document](db, src).On(on),
		"no source":          momo.Merge[suite.Document](db, nil).On(on).WhenMatchedUpdate(suite.DocTitle.Set("x")),
		"empty update":       momo.Merge[suite.Document](db, src).On(on).WhenMatchedUpdate(),
		"pk update":          momo.Merge[suite.Document](db, src).On(on).WhenMatchedUpdate(suite.DocID.Set(1)),
		"version update":     momo.Merge[suite.Document](db, src).On(on).WhenMatchedUpdate(suite.DocVersion.Set(1)),
		"duplicate assign":   momo.Merge[suite.Document](db, src).On(on).WhenMatchedUpdate(suite.DocTitle.Set("a"), suite.DocTitle.Set("b")),
		"excluded":           momo.Merge[suite.Document](db, src).On(on).WhenMatchedUpdate(suite.DocTitle.SetExcluded()),
		"value count":        momo.Merge[suite.Document](db, src).On(on).WhenNotMatchedInsert(cols),
		"no insert columns":  momo.Merge[suite.Document](db, src).On(on).WhenNotMatchedInsert(nil),
		"duplicate column":   momo.Merge[suite.Document](db, src).On(on).WhenNotMatchedInsert([]momo.ColumnRef{suite.DocTitle, suite.DocTitle}, momo.TypedSQL[string]("src.t"), momo.TypedSQL[string]("src.t")),
		"returning unmapped": momo.Merge[suite.Document](db, src).On(on).WhenMatchedUpdate(suite.DocTitle.Set("x")).Returning(momo.NewColumn[int64]("documents", "nope", 0)),
	}
	for name, q := range cases {
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: SQL = %v, want ErrValidation", name, err)
		}
	}
}

// Catches: the version gate not running before the statement, the RETURNING
// threshold being wrong, and a RETURNING MERGE reaching an old server.
func TestMergeServerVersionGate(t *testing.T) {
	cases := []struct {
		version   int
		returning bool
		ok        bool
	}{
		{140013, false, false},
		{150000, false, true},
		{150000, true, false},
		{160004, true, false},
		{170000, true, true},
		{170002, false, true},
	}
	for _, c := range cases {
		drv := newVersioned(c.version)
		drv.SetRows([][]any{{int64(4)}})
		db := momo.New(drv)
		q := mergeInsertQuery(db)
		var err error
		if c.returning {
			var rows *momo.ModelRows[suite.Document]
			rows, err = q.Returning(suite.DocID).Rows(t.Context())
			if err == nil {
				var m suite.Document
				if !rows.Next() || rows.ScanModel(&m) != nil || m.ID != 4 {
					t.Errorf("version %d: RETURNING row = %+v", c.version, m)
				}
				_ = rows.Close()
			}
		} else {
			_, err = q.Exec(t.Context())
		}
		if (err == nil) != c.ok {
			t.Errorf("version %d returning %v: err = %v, want ok=%v", c.version, c.returning, err, c.ok)
			continue
		}
		if err != nil && !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("version %d returning %v: err %v is not ErrUnsupported", c.version, c.returning, err)
		}
		if ran := len(drv.Calls()); ran != map[bool]int{true: 1, false: 0}[c.ok] {
			t.Errorf("version %d returning %v: %d statements ran, want %v", c.version, c.returning, ran, c.ok)
		}
	}
}

// Catches: a driver that cannot report its version letting MERGE through
// unchecked.
func TestMergeFailsClosedWithoutServerVersioner(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	if _, err := mergeInsertQuery(db).Exec(t.Context()); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Exec = %v, want ErrUnsupported", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("calls = %v, want none", drv.Calls())
	}
}

// Catches: the version being read from the pool while a transaction holds the
// connection (a deadlock on a one-connection pool), or read from another session.
func TestMergeReadsTheVersionOnTheTransaction(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{170000}})
	db := momo.New(drv)
	err := db.Tx(t.Context(), momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := mergeInsertQuery(tx).Exec(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 2 || !strings.Contains(calls[0].SQL, "server_version_num") || !strings.HasPrefix(calls[1].SQL, "MERGE INTO") {
		t.Fatalf("calls = %+v, want the version query then the MERGE", calls)
	}
}

// Catches: a sensitive value bound by MERGE reaching a hook.
func TestMergeMasksSensitiveArgs(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := newVersioned(170000)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	src := momo.RawSubquery("SELECT 3 AS id")
	_, err := momo.Merge[suite.Document](db, src).
		On(suite.DocID.EqExpr(momo.TypedSQL[int64]("src.id"))).
		WhenMatchedUpdate(suite.DocSecret.Set(sensitiveValue)).
		WhenNotMatchedInsert([]momo.ColumnRef{suite.DocID, suite.DocSecret},
			momo.TypedSQL[int64]("src.id"), momo.TypedSQL[string]("?", sensitiveValue)).
		Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv.Driver)
	for _, c := range drv.Calls() {
		if n := countArg(c.Args, sensitiveValue); n != 2 {
			t.Errorf("driver got the sensitive value %d times, want 2 (update and insert)", n)
		}
	}
}

func countArg(args []any, v any) int {
	n := 0
	for _, a := range args {
		if a == v {
			n++
		}
	}
	return n
}

func bulkQ(q momo.Querier, rows ...[]any) momo.UpdateQuery[suite.Document] {
	return momo.BulkUpdate[suite.Document](q, suite.DocID, []momo.ColumnRef{suite.DocTitle}, rows)
}

// Catches: values inlined into the SQL, a join that does not key on the
// listed column (touching rows outside the list), and a lost version bump.
func TestBulkUpdateRendersKeyedValuesList(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	c, err := bulkQ(db, []any{int64(2), "new"}, []any{int64(3), "other"}).Where(suite.DocTenantID.Eq(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	hasAll(t, c.SQL,
		`UPDATE "documents" SET "title" = "momo_v"."title"`,
		`"version" = "documents"."version" + 1`,
		` FROM (SELECT "id", "title" FROM "documents" WHERE false UNION ALL VALUES ($`,
		`) AS "momo_v" WHERE "documents"."id" = "momo_v"."id" AND `)
	if strings.Contains(c.SQL, "new") || strings.Contains(c.SQL, "other") {
		t.Errorf("a value was inlined:\n%s", c.SQL)
	}
	for _, want := range []any{int64(2), "new", int64(3), "other", int64(1)} {
		if !slices.Contains(c.Args, want) {
			t.Errorf("args %v lack %v", c.Args, want)
		}
	}
}

// Catches: BulkUpdate rendering on a dialect without it.
func TestBulkUpdateUnsupportedOnSQLite(t *testing.T) {
	drv := fakedriver.New(dialect.SQLite)
	q := bulkQ(momo.New(drv), []any{int64(2), "new"})
	if _, err := q.SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("SQL = %v, want ErrUnsupported", err)
	}
	if _, err := q.Exec(t.Context()); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Exec = %v, want ErrUnsupported", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("calls = %v, want none", drv.Calls())
	}
}

// Catches: malformed lists reaching the database, and BulkUpdate being allowed
// to write the columns Update guards.
func TestBulkUpdateValidation(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	cols := []momo.ColumnRef{suite.DocTitle}
	row := []any{int64(2), "new"}
	cases := map[string]momo.UpdateQuery[suite.Document]{
		"no rows":        bulkQ(db),
		"short row":      bulkQ(db, []any{int64(2)}),
		"long row":       bulkQ(db, []any{int64(2), "a", "b"}),
		"no columns":     momo.BulkUpdate[suite.Document](db, suite.DocID, nil, [][]any{{int64(2)}}),
		"nil key":        momo.BulkUpdate[suite.Document](db, nil, cols, [][]any{row}),
		"key as column":  momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocID}, [][]any{{int64(2), int64(2)}}),
		"version column": momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocVersion}, [][]any{{int64(2), int64(5)}}),
		"duplicate":      momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocTitle, suite.DocTitle}, [][]any{{int64(2), "a", "b"}}),
		"with set":       bulkQ(db, row).Set(suite.DocTenantID.Set(1)),
		"unmapped":       momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{momo.NewColumn[string]("documents", "nope", 0)}, [][]any{row}),
	}
	for name, q := range cases {
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: SQL = %v, want ErrValidation", name, err)
		}
	}
}

// Catches: a sensitive value in a bulk update reaching a hook.
func TestBulkUpdateMasksSensitiveArgs(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	_, err := momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocSecret},
		[][]any{{int64(1), sensitiveValue}}).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
}

// Catches: Truncate on PostgreSQL deleting row by row, and on SQLite leaving
// the autoincrement counter behind.
func TestTruncateStatements(t *testing.T) {
	pg := fakedriver.New(dialect.Postgres)
	if err := momo.Truncate[suite.Document](t.Context(), momo.New(pg)); err != nil {
		t.Fatal(err)
	}
	if c := pg.Calls(); len(c) != 1 || c[0].SQL != `TRUNCATE TABLE "documents"` {
		t.Fatalf("postgres calls = %+v", c)
	}

	sq := fakedriver.New(dialect.SQLite)
	if err := momo.Truncate[suite.Document](t.Context(), momo.New(sq)); err != nil {
		t.Fatal(err)
	}
	if c := sq.Calls(); len(c) != 2 || c[0].SQL != `DELETE FROM "documents"` || !strings.Contains(c[1].SQL, "sqlite_master") {
		t.Fatalf("sqlite without sequence calls = %+v", c)
	}

	seq := fakedriver.New(dialect.SQLite)
	seq.SetRows([][]any{{int64(1)}})
	if err := momo.Truncate[suite.Document](t.Context(), momo.New(seq)); err != nil {
		t.Fatal(err)
	}
	c := seq.Calls()
	if len(c) != 3 || !strings.HasPrefix(c[2].SQL, "DELETE FROM sqlite_sequence WHERE name = ") || !slices.Equal(c[2].Args, []any{"documents"}) {
		t.Fatalf("sqlite with sequence calls = %+v", c)
	}
	if seq.Commits() != 1 {
		t.Fatalf("commits = %d, want the two statements in one transaction", seq.Commits())
	}
}
