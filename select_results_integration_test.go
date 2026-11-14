// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestResultHelpers(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for i := 0; i < 7; i++ {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", string(rune('a'+i))); err != nil {
				t.Fatal(err)
			}
		}
		titles, err := momo.Pluck(ctx, momo.Select[suite.Document](db).OrderBy(suite.DocID.Asc()), suite.DocTitle)
		if err != nil || len(titles) != 7 || titles[0] != "a" {
			t.Fatalf("Pluck = %v %v", titles, err)
		}
		ids, err := momo.IDs(ctx, momo.Select[suite.Document](db))
		if err != nil || len(ids) != 7 {
			t.Fatalf("IDs = %v %v", ids, err)
		}
		first, err := momo.FirstID(ctx, momo.Select[suite.Document](db).OrderBy(suite.DocID.Asc()))
		if err != nil || first != ids[0] {
			t.Fatalf("FirstID = %d %v, want %d", first, err, ids[0])
		}
		page, total, err := momo.FindAndCount(ctx, momo.Select[suite.Document](db).OrderBy(suite.DocID.Asc()).Limit(2))
		if err != nil || len(page) != 2 || total != 7 {
			t.Fatalf("FindAndCount page=%d total=%d err=%v", len(page), total, err)
		}
		seen := 0
		err = momo.Batches(ctx, momo.Select[suite.Document](db), 3, func(b []suite.Document) error {
			seen += len(b)
			return nil
		})
		if err != nil || seen != 7 {
			t.Fatalf("Batches visited %d rows, err=%v", seen, err)
		}
		maps, err := momo.MapRows(ctx, db, "SELECT title FROM documents ORDER BY id LIMIT 1")
		if err != nil || len(maps) != 1 || maps[0]["title"] != "a" {
			t.Fatalf("MapRows = %v %v", maps, err)
		}
		vals, err := momo.Scalars[string](ctx, db, "SELECT title FROM documents WHERE tenant_id = ? ORDER BY id", 1)
		if err != nil || len(vals) != 7 || vals[6] != "g" {
			t.Fatalf("Scalars = %v %v", vals, err)
		}
	})
}

// Rows inserted with higher keys while Batches runs are each visited exactly
// once, and no row is skipped or repeated. OFFSET paging would repeat rows
// after a delete or skip them; keyset paging on the primary key cannot.
func TestBatchesUnderConcurrentInserts(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for i := 0; i < 6; i++ {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", fmt.Sprintf("seed-%d", i)); err != nil {
				t.Fatal(err)
			}
		}
		visits := map[int64]int{}
		inserted := false
		err := momo.Batches(ctx, momo.Select[suite.Document](db), 2, func(b []suite.Document) error {
			for _, m := range b {
				visits[m.ID]++
			}
			if !inserted {
				inserted = true
				for i := 0; i < 3; i++ {
					if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", fmt.Sprintf("late-%d", i)); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(visits) != 9 {
			t.Fatalf("visited %d distinct rows, want 9 (6 seeded + 3 inserted during the run)", len(visits))
		}
		for id, n := range visits {
			if n != 1 {
				t.Fatalf("row %d visited %d times", id, n)
			}
		}
	})
}
