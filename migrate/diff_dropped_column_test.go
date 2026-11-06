// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// TestDiffSkipsDropsOfObjectsOnDroppedColumn: dropping a column drops the
// indexes and constraints on it, so an explicit DROP INDEX or DROP CONSTRAINT
// after DROP COLUMN fails (42704). Worse, with a concurrent index the head
// would commit the DROP COLUMN and the no-transaction tail would then fail,
// leaving the migration dirty. Diff must not emit those drops.
func TestDiffSkipsDropsOfObjectsOnDroppedColumn(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		name := map[bool]string{false: "plain", true: "concurrent"}[concurrent]
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			from := docsSchema()
			from.Tables[0].Indexes[0].Concurrently = concurrent
			from.Tables[0].Indexes = append(from.Tables[0].Indexes, schema.Index{
				Name: "docs_lower_title", Parts: []schema.IndexPart{{Expr: "lower(title)"}},
			})
			from.Tables[0].Uniques = []schema.Unique{{Name: "docs_title_key", Columns: []string{"title"}}}
			from.Tables[0].Checks = []schema.Check{{Name: "docs_title_ne", Expr: "title <> 'x'"}}
			first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
			if err != nil || m1 == nil {
				t.Fatal(m1, err)
			}
			to := docsSchema()
			to.Tables[0].Columns = to.Tables[0].Columns[:1] // drop title
			to.Tables[0].Indexes = nil
			e := offlineEngine(t, mapFS(m1.Files), lockfreeClock)
			m, err := e.Diff(ctx, to, "drop_title", migrate.DiffOptions{AllowDestructive: true})
			if err != nil || m == nil {
				t.Fatal(m, err)
			}
			if m.Next != nil || m.NoTx {
				t.Fatalf("a plain column drop must stay one transactional file, got NoTx=%v Next=%v", m.NoTx, m.Next)
			}
			up := string(m.Files[m.Version+"_drop_title.up.sql"])
			if !strings.Contains(up, `DROP COLUMN "title"`) {
				t.Fatalf("column drop missing:\n%s", up)
			}
			for _, bad := range []string{"DROP INDEX", "DROP CONSTRAINT"} {
				if strings.Contains(up, bad) {
					t.Fatalf("%s on the dropped column must be left to DROP COLUMN:\n%s", bad, up)
				}
			}
		})
	}
}

// TestDiffSkipsPrimaryKeyDropOnDroppedColumn: the same holds for a primary key
// whose column is dropped; the new key is still added.
func TestDiffSkipsPrimaryKeyDropOnDroppedColumn(t *testing.T) {
	ctx := context.Background()
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	to := docsSchema()
	to.Tables[0].Columns = to.Tables[0].Columns[1:] // drop id
	to.Tables[0].PrimaryKey = []string{"title"}
	to.Tables[0].Indexes = nil
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, to, "rekey", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	up := string(m.Files[m.Version+"_rekey.up.sql"])
	if strings.Contains(up, "DROP CONSTRAINT") || !strings.Contains(up, `PRIMARY KEY ("title")`) {
		t.Fatalf("want DROP COLUMN to take the old key and a new key added:\n%s", up)
	}
}

// TestDiffKeepsDropsOfOtherColumnObjects: the skip only covers objects that
// mention a dropped column.
func TestDiffKeepsDropsOfOtherColumnObjects(t *testing.T) {
	ctx := context.Background()
	from := docsSchema()
	from.Tables[0].Indexes = append(from.Tables[0].Indexes, schema.Index{Name: "docs_id_idx", Parts: []schema.IndexPart{{Column: "id"}}})
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	to := docsSchema()
	to.Tables[0].Columns = to.Tables[0].Columns[:1]
	to.Tables[0].Indexes = nil
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, to, "drop_title", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	up := string(m.Files[m.Version+"_drop_title.up.sql"])
	if !strings.Contains(up, `DROP INDEX "docs_id_idx"`) || strings.Contains(up, `DROP INDEX "docs_title"`) {
		t.Fatalf("only the index on the kept column is dropped explicitly:\n%s", up)
	}
}

// TestDiffAddUniqueOnPartitionedTable: PostgreSQL rejects CREATE INDEX
// CONCURRENTLY on a partitioned table, so a unique added there is a plain
// ADD CONSTRAINT in the transactional file.
func TestDiffAddUniqueOnPartitionedTable(t *testing.T) {
	ctx := context.Background()
	from := docsSchema()
	from.Tables[0].Partition = &schema.Partition{Strategy: "range", Key: "RANGE (id)"}
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	to := docsSchema()
	to.Tables[0].Partition = from.Tables[0].Partition
	to.Tables[0].Uniques = []schema.Unique{{Name: "docs_id_title_key", Columns: []string{"id", "title"}}}
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, to, "uq", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	up := string(m.Files[m.Version+"_uq.up.sql"])
	if m.Next != nil || m.NoTx || strings.Contains(up, "CONCURRENTLY") || !strings.Contains(up, `ADD CONSTRAINT "docs_id_title_key" UNIQUE ("id", "title")`) {
		t.Fatalf("want a plain transactional ADD CONSTRAINT UNIQUE:\n%s", up)
	}
}
