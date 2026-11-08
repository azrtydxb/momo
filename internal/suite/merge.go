// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

func titleOf(t *testing.T, db *momo.DB, id int64) (string, int64) {
	t.Helper()
	rows, err := momo.Select[Document](db).Where(DocID.Eq(id)).All(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("suite: row %d: %d rows, %v", id, len(rows), err)
	}
	return rows[0].Title, rows[0].Version
}

func countTitle(t *testing.T, db *momo.DB, title string) int64 {
	t.Helper()
	n, err := momo.Select[Document](db).Where(DocTitle.Eq(title)).Count(context.Background())
	if err != nil {
		t.Fatalf("suite: count %q: %v", title, err)
	}
	return n
}

// MergeAndBulkUpdate runs MERGE, the bulk update from a VALUES list and
// Truncate. On a dialect without Merge it asserts ErrUnsupported for MERGE and
// the bulk update; on PostgreSQL it asserts the row results and the server
// version gate (RETURNING on MERGE needs PostgreSQL 17). Truncate runs on every
// dialect.
func MergeAndBulkUpdate(t *testing.T, db *momo.DB, caps Caps) {
	t.Helper()
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'one'), (1, 'two')"); err != nil {
		t.Fatal(err)
	}
	bulk := func(rows ...[]any) momo.UpdateQuery[Document] {
		return momo.BulkUpdate[Document](db, DocID, []momo.ColumnRef{DocTitle}, rows)
	}
	merged := func(id int64, title string) momo.MergeQuery[Document] {
		src := momo.RawSubquery("SELECT ?::bigint AS id, 1 AS tenant_id, ?::text AS title", id, title)
		return momo.Merge[Document](db, src).
			On(DocID.EqExpr(momo.TypedSQL[int64]("src.id"))).
			WhenNotMatchedInsert([]momo.ColumnRef{DocID, DocTenantID, DocTitle},
				momo.TypedSQL[int64]("src.id"), momo.TypedSQL[int64]("src.tenant_id"), momo.TypedSQL[string]("src.title"))
	}

	if !caps.Merge {
		if _, err := bulk([]any{int64(2), "new"}).SQL(); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("suite: BulkUpdate.SQL on %s = %v, want ErrUnsupported", caps.Dialect, err)
		}
		if _, err := bulk([]any{int64(2), "new"}).Exec(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("suite: BulkUpdate.Exec on %s = %v, want ErrUnsupported", caps.Dialect, err)
		}
		if _, err := merged(3, "merged").Exec(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("suite: Merge.Exec on %s = %v, want ErrUnsupported", caps.Dialect, err)
		}
		if title, _ := titleOf(t, db, 2); title != "two" {
			t.Fatalf("suite: an unsupported statement changed row 2 to %q", title)
		}
		truncateAndCount(t, db)
		return
	}

	// Bulk update: only the listed keys change, a key that is not in the table
	// changes nothing.
	res, err := bulk([]any{int64(2), "new"}).Exec(ctx)
	if err != nil || res.RowsAffected != 1 {
		t.Fatalf("suite: BulkUpdate = %+v, %v, want 1 row", res, err)
	}
	if title, version := titleOf(t, db, 1); title != "one" || version != 1 {
		t.Fatalf("suite: row 1 outside the VALUES list changed: %q v%d", title, version)
	}
	if title, version := titleOf(t, db, 2); title != "new" || version != 2 {
		t.Fatalf("suite: row 2 = %q v%d, want new v2", title, version)
	}
	res, err = bulk([]any{int64(1), "uno"}, []any{int64(99), "ghost"}).Exec(ctx)
	if err != nil || res.RowsAffected != 1 {
		t.Fatalf("suite: BulkUpdate with a missing key = %+v, %v, want 1 row", res, err)
	}
	if title, _ := titleOf(t, db, 1); title != "uno" {
		t.Fatalf("suite: row 1 = %q, want uno", title)
	}
	if n := countTitle(t, db, "ghost"); n != 0 {
		t.Fatalf("suite: a key outside the table created %d rows", n)
	}

	// MERGE: the version the driver reports decides what the server accepts.
	sv, ok := db.Driver().(momo.ServerVersioner)
	if !ok {
		t.Fatal("suite: the PostgreSQL driver must implement momo.ServerVersioner")
	}
	ver, err := sv.ServerVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := merged(3, "merged").Exec(ctx); err != nil {
		t.Fatalf("suite: Merge insert on server %d: %v", ver, err)
	}
	if n := countTitle(t, db, "merged"); n != 1 {
		t.Fatalf("suite: %d rows titled merged, want 1", n)
	}
	src := momo.RawSubquery("SELECT 1 AS id, 'matched' AS title")
	res, err = momo.Merge[Document](db, src).
		On(DocID.EqExpr(momo.TypedSQL[int64]("src.id"))).
		WhenMatchedUpdate(DocTitle.SetExpr(momo.TypedSQL[string]("src.title"))).
		Exec(ctx)
	if err != nil || res.RowsAffected != 1 {
		t.Fatalf("suite: Merge update = %+v, %v, want 1 row", res, err)
	}
	if title, _ := titleOf(t, db, 1); title != "matched" {
		t.Fatalf("suite: row 1 = %q, want matched", title)
	}

	rows, err := merged(4, "merged-2").Returning(DocID).Rows(ctx)
	if ver < 170000 {
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("suite: Merge RETURNING on server %d = %v, want ErrUnsupported", ver, err)
		}
		if n := countTitle(t, db, "merged-2"); n != 0 {
			t.Fatalf("suite: a refused MERGE RETURNING inserted %d rows", n)
		}
	} else {
		if err != nil {
			t.Fatalf("suite: Merge RETURNING on server %d: %v", ver, err)
		}
		var ids []int64
		for rows.Next() {
			var m Document
			if err := rows.ScanModel(&m); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, m.ID)
		}
		if err := rows.Close(); err != nil || len(ids) != 1 || ids[0] != 4 {
			t.Fatalf("suite: Merge RETURNING ids = %v, close %v, want [4]", ids, err)
		}
	}
	truncateAndCount(t, db)
}

func truncateAndCount(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	if err := momo.Truncate[Document](ctx, db); err != nil {
		t.Fatal(err)
	}
	if n, err := momo.Select[Document](db).Count(ctx); err != nil || n != 0 {
		t.Fatalf("suite: after Truncate count = %d, %v, want 0", n, err)
	}
}
