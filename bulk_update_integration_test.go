// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// bulkClock is the fixed statement clock of the integration tests.
var bulkClock = time.Date(2031, 1, 2, 3, 4, 5, 123000000, time.UTC)

// bulkLimit leaves room for one stock row or two document rows per statement,
// so every multi-row list is chunked.
const bulkLimit = 5

func eachBulk(t *testing.T, fn func(t *testing.T, db *momo.DB)) {
	t.Helper()
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, _ dialect.Name) { fn(t, db) },
		momo.WithMaxParams(bulkLimit), momo.WithClock(func() time.Time { return bulkClock }))
}

func createBulkStock(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	region, note := "text", "text"
	count := "bigint"
	if db.Dialect().Name == dialect.MySQL {
		region, note = "varchar(64)", "varchar(64)"
	}
	if db.Dialect().Name == dialect.SQLite {
		count = "integer"
	}
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS bulk_stock",
		"CREATE TABLE bulk_stock (region " + region + " not null, sku " + count + " not null, qty " + count + " not null, note " + note + ", primary key (region, sku))",
		"INSERT INTO bulk_stock (region, sku, qty, note) VALUES ('eu', 1, 10, 'a'), ('eu', 2, 20, 'b'), ('us', 1, 30, NULL), ('us', 2, 40, 'd')",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS bulk_stock") })
}

type stockRow struct {
	qty  int64
	note *string
}

func readStock(t testing.TB, q momo.Querier) map[string]stockRow {
	t.Helper()
	rows, err := momo.Select[bulkStock](q).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]stockRow{}
	for _, r := range rows {
		out[r.Region+string(rune('0'+r.SKU))] = stockRow{r.Qty, r.Note}
	}
	return out
}

func wantStock(t testing.TB, got map[string]stockRow, key string, qty int64, note *string) {
	t.Helper()
	g, ok := got[key]
	if !ok || g.qty != qty || (g.note == nil) != (note == nil) || (note != nil && *g.note != *note) {
		t.Errorf("row %s = %+v (present %v), want qty %d note %v", key, g, ok, qty, note)
	}
}

func strp(s string) *string { return &s }

// Catches: a composite key matched on one column (rows of the same region or
// sku changing), a NULL value bound as a zero value, a key outside the table
// creating a row, a list-wide affected count that differs between servers, and
// a chunked list losing a row at a chunk boundary.
func TestBulkUpdateKeysCompositeAndNullIntegration(t *testing.T) {
	eachBulk(t, func(t *testing.T, db *momo.DB) {
		createBulkStock(t, db)
		ctx := context.Background()
		res, err := momo.BulkUpdateKeys[bulkStock](db, []momo.ColumnRef{bulkRegion, bulkSKU}, []momo.ColumnRef{bulkQty, bulkNote}, [][]any{
			{"eu", int64(1), int64(11), nil},
			{"us", int64(1), int64(31), "n"},
			{"zz", int64(9), int64(99), "ghost"},
			{"us", int64(2), int64(40), "d"}, // same values as stored
		}).Exec(ctx)
		if err != nil || res.RowsAffected != 3 {
			t.Fatalf("BulkUpdateKeys = %+v, %v, want 3 rows matched (a same-value row counts)", res, err)
		}
		got := readStock(t, db)
		if len(got) != 4 {
			t.Fatalf("%d rows, want 4 (a key outside the table must not insert)", len(got))
		}
		wantStock(t, got, "eu1", 11, nil)
		wantStock(t, got, "eu2", 20, strp("b"))
		wantStock(t, got, "us1", 31, strp("n"))
		wantStock(t, got, "us2", 40, strp("d"))
	})
}

// Catches: the single-key BulkUpdate regressing on a dialect, and a same-value
// row not counted as affected (MySQL without clientFoundRows).
func TestBulkUpdateAffectedRowsCountsMatchedIntegration(t *testing.T) {
	eachBulk(t, func(t *testing.T, db *momo.DB) {
		createBulkStock(t, db)
		res, err := momo.BulkUpdate[bulkStock](db, bulkSKU, []momo.ColumnRef{bulkQty}, [][]any{{int64(1), int64(10)}}).
			Where(bulkRegion.Eq("eu")).Exec(context.Background())
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("BulkUpdate = %+v, %v, want 1 row", res, err)
		}
		got := readStock(t, db)
		wantStock(t, got, "eu1", 10, strp("a"))
		wantStock(t, got, "us1", 30, nil) // the Where kept the other region out
	})
}

// Catches: a failing later chunk leaving the earlier chunks applied, on a DB
// (own transaction) and on a Tx (savepoint), and the Tx unusable afterwards.
func TestBulkUpdateChunkFailureRollsBackIntegration(t *testing.T) {
	eachBulk(t, func(t *testing.T, db *momo.DB) {
		createBulkStock(t, db)
		ctx := context.Background()
		// One row per statement; the second chunk writes NULL to a NOT NULL column.
		failing := func(q momo.Querier) momo.UpdateQuery[bulkStock] {
			return momo.BulkUpdateKeys[bulkStock](q, []momo.ColumnRef{bulkRegion, bulkSKU}, []momo.ColumnRef{bulkQty}, [][]any{
				{"eu", int64(1), int64(111)},
				{"eu", int64(2), nil},
			})
		}
		if _, err := failing(db).Exec(ctx); err == nil {
			t.Fatal("Exec with a NULL in a NOT NULL column succeeded")
		}
		wantStock(t, readStock(t, db), "eu1", 10, strp("a"))

		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := momo.BulkUpdateKeys[bulkStock](tx, []momo.ColumnRef{bulkRegion, bulkSKU}, []momo.ColumnRef{bulkQty},
				[][]any{{"us", int64(1), int64(301)}}).Exec(ctx); err != nil {
				return err
			}
			if _, err := failing(tx).Exec(ctx); err == nil {
				return errors.New("Exec in a Tx with a NULL in a NOT NULL column succeeded")
			}
			got := readStock(t, tx)
			wantStock(t, got, "eu1", 10, strp("a"))
			wantStock(t, got, "us1", 301, nil) // the earlier statement of the Tx stays
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		wantStock(t, readStock(t, db), "us1", 301, nil)
	})
}

// Catches: RETURNING reaching MySQL and MariaDB (no UPDATE ... RETURNING),
// missing from PostgreSQL and SQLite, or returning rows outside the list.
func TestBulkUpdateReturningIntegration(t *testing.T) {
	eachBulk(t, func(t *testing.T, db *momo.DB) {
		createBulkStock(t, db)
		ctx := context.Background()
		q := momo.BulkUpdateKeys[bulkStock](db, []momo.ColumnRef{bulkRegion, bulkSKU}, []momo.ColumnRef{bulkQty},
			[][]any{{"eu", int64(2), int64(22)}}).Returning(bulkRegion, bulkSKU, bulkQty)
		rows, err := q.Rows(ctx)
		if db.Dialect().Name == dialect.MySQL {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("Rows on the MySQL family = %v, want ErrUnsupported", err)
			}
			wantStock(t, readStock(t, db), "eu2", 20, strp("b"))
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		var got []bulkStock
		for rows.Next() {
			var m bulkStock
			if err := rows.ScanModel(&m); err != nil {
				t.Fatal(err)
			}
			got = append(got, m)
		}
		if err := rows.Close(); err != nil || len(got) != 1 || got[0].Region != "eu" || got[0].SKU != 2 || got[0].Qty != 22 {
			t.Fatalf("returned %+v, close %v, want eu/2 qty 22", got, err)
		}
	})
}

func docState(t testing.TB, q momo.Querier, id int64) suite.Document {
	t.Helper()
	rows, err := momo.Select[suite.Document](q).WithDeleted().Where(suite.DocID.Eq(id)).All(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("document %d: %d rows, %v", id, len(rows), err)
	}
	return rows[0]
}

// Catches: updated_at taken from the database or differing between chunks
// (it comes from the statement clock), a lost version bump, an explicit
// updated_at accepted, and the soft-delete scope: a deleted row matched
// unless Where leaves it out, as in Update.
func TestBulkUpdateTimestampVersionAndSoftDeleteIntegration(t *testing.T) {
	eachBulk(t, func(t *testing.T, db *momo.DB) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'a'), (1, 'b'), (1, 'c'), (1, 'd')"); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.SoftDelete[suite.Document](db).Where(suite.DocID.Eq(4)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		deleted := docState(t, db, 4)
		if deleted.DeletedAt == nil {
			t.Fatal("document 4 is not soft-deleted")
		}
		rows := [][]any{{int64(1), "a2"}, {int64(2), "b2"}, {int64(3), "c2"}, {int64(4), "d2"}}
		title := []momo.ColumnRef{suite.DocTitle}

		res, err := momo.BulkUpdate[suite.Document](db, suite.DocID, title, rows).Where(suite.DocDeletedAt.IsNull()).Exec(ctx)
		if err != nil || res.RowsAffected != 3 {
			t.Fatalf("BulkUpdate = %+v, %v, want 3 rows (4 listed, 1 soft-deleted)", res, err)
		}
		for id, want := range map[int64]string{1: "a2", 2: "b2", 3: "c2", 4: "d"} {
			d := docState(t, db, id)
			if d.Title != want {
				t.Errorf("document %d title = %q, want %q", id, d.Title, want)
			}
			if id < 4 && (d.Version != 2 || !d.UpdatedAt.Equal(bulkClock)) {
				t.Errorf("document %d version %d updated_at %v, want 2 and %v (one statement clock across chunks)", id, d.Version, d.UpdatedAt, bulkClock)
			}
		}
		if d := docState(t, db, 4); d.Version != deleted.Version || d.DeletedAt == nil {
			t.Errorf("soft-deleted document = %+v, want untouched", d)
		}

		// Without a Where the soft-deleted row matches, as in Update.
		res, err = momo.BulkUpdate[suite.Document](db, suite.DocID, title, rows[3:]).Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("BulkUpdate over a soft-deleted row = %+v, %v, want 1", res, err)
		}
		if d := docState(t, db, 4); d.Title != "d2" || d.DeletedAt == nil {
			t.Errorf("document 4 = %+v, want title d2 and still soft-deleted", d)
		}

		// updated_at is the statement's, never the caller's.
		_, err = momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocUpdatedAt},
			[][]any{{int64(1), bulkClock}}).Exec(ctx)
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("explicit updated_at = %v, want ErrValidation", err)
		}
	})
}
