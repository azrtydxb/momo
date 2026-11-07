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
			// Objects named by column are removed with the column. Objects that
			// only hold an expression may or may not mention it, so they get an
			// idempotent drop that is right in both cases.
			for _, bad := range []string{`DROP INDEX "`, `DROP CONSTRAINT "`, `"docs_title"`, `"docs_title_key"`} {
				if strings.Contains(up, bad) {
					t.Fatalf("%s on the dropped column must be left to DROP COLUMN:\n%s", bad, up)
				}
			}
			for _, want := range []string{`DROP INDEX IF EXISTS "docs_lower_title"`, `DROP CONSTRAINT IF EXISTS "docs_title_ne"`} {
				if !strings.Contains(up, want) {
					t.Fatalf("expression object needs %s:\n%s", want, up)
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

// TestDiffExpressionDropsAreIdempotent: whether an expression mentions a
// dropped column cannot be told reliably (string literals, function and type
// names, quoted identifiers). Guessing wrong either leaves the constraint in
// the database or fails the migration, so a drop of an expression-only object
// on a table that loses columns is DROP ... IF EXISTS.
func TestDiffExpressionDropsAreIdempotent(t *testing.T) {
	cases := []struct {
		name, col, expr string
	}{
		{"literal", "note", "id <> length('note')"},
		{"function name", "length", "id > length('x')"},
		{"cast", "date", "id::date IS NOT NULL"},
		{"quoted identifier", "my title", `"my title" <> ''`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			from := docsSchema()
			from.Tables[0].Columns = append(from.Tables[0].Columns, schema.Column{Name: tc.col, Type: "TEXT", Default: "''"})
			from.Tables[0].Checks = []schema.Check{{Name: "docs_chk", Expr: tc.expr}}
			first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
			if err != nil || m1 == nil {
				t.Fatal(m1, err)
			}
			m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, docsSchema(), "drop_col", migrate.DiffOptions{AllowDestructive: true})
			if err != nil || m == nil {
				t.Fatal(m, err)
			}
			up := string(m.Files[m.Version+"_drop_col.up.sql"])
			if !strings.Contains(up, `DROP CONSTRAINT IF EXISTS "docs_chk"`) {
				t.Fatalf("want an idempotent explicit drop:\n%s", up)
			}
		})
	}
}

// TestDiffPartitionedTableIndexesAreNotConcurrent: PostgreSQL rejects CREATE
// and DROP INDEX CONCURRENTLY on a partitioned table, and in the
// no-transaction tail that would fail after the head committed. They stay
// plain statements in the transactional head.
func TestDiffPartitionedTableIndexesAreNotConcurrent(t *testing.T) {
	ctx := context.Background()
	part := &schema.Partition{Strategy: "range", Key: "RANGE (id)"}
	from := docsSchema()
	from.Tables[0].Partition = part
	from.Tables[0].Indexes = append(from.Tables[0].Indexes, schema.Index{
		Name: "docs_old", Concurrently: true, Parts: []schema.IndexPart{{Column: "title"}, {Column: "id"}},
	})
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	to := docsSchema()
	to.Tables[0].Partition = part
	to.Tables[0].Indexes = append(to.Tables[0].Indexes, schema.Index{
		Name: "docs_new", Concurrently: true, Parts: []schema.IndexPart{{Column: "id"}, {Column: "title"}},
	})
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, to, "swap", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	up := string(m.Files[m.Version+"_swap.up.sql"])
	if m.Next != nil || m.NoTx || strings.Contains(up, "CONCURRENTLY") ||
		!strings.Contains(up, `CREATE INDEX "docs_new"`) || !strings.Contains(up, `DROP INDEX "docs_old"`) {
		t.Fatalf("want plain transactional index statements:\n%s", up)
	}
}

// TestDiffPlainDropOfObjectsUnrelatedToDroppedColumn: a check on another column
// is dropped plainly, with no IF EXISTS, even when the diff drops a column.
func TestDiffPlainDropOfObjectsUnrelatedToDroppedColumn(t *testing.T) {
	ctx := context.Background()
	from := docsSchema()
	from.Tables[0].Columns = append(from.Tables[0].Columns, schema.Column{Name: "note", Type: "TEXT", Default: "''"})
	from.Tables[0].Checks = []schema.Check{{Name: "docs_id_pos", Expr: "id > 0"}}
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, docsSchema(), "drop_note", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	up := string(m.Files[m.Version+"_drop_note.up.sql"])
	if !strings.Contains(up, `DROP CONSTRAINT "docs_id_pos"`) || strings.Contains(up, "IF EXISTS") {
		t.Fatalf("want a plain DROP CONSTRAINT for the unrelated check:\n%s", up)
	}
}

// TestDiffPartialIndexOnDroppedColumnIsSkipped: an index that lists a dropped
// column goes with it even when its WHERE only mentions another column.
func TestDiffPartialIndexOnDroppedColumnIsSkipped(t *testing.T) {
	ctx := context.Background()
	from := docsSchema()
	from.Tables[0].Indexes = []schema.Index{{Name: "docs_title_part", Where: "id > 0", Parts: []schema.IndexPart{{Column: "title"}}}}
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
	if strings.Contains(up, "docs_title_part") || !strings.Contains(up, `DROP COLUMN "title"`) {
		t.Fatalf("the partial index must be left to DROP COLUMN:\n%s", up)
	}
}

// TestDiffConcurrentExpressionIndexOnDroppedColumn: a concurrent expression
// index that mentions the dropped column keeps its drop, written
// DROP INDEX CONCURRENTLY IF EXISTS in the no-transaction tail, after the head
// has dropped the column.
func TestDiffConcurrentExpressionIndexOnDroppedColumn(t *testing.T) {
	ctx := context.Background()
	from := docsSchema()
	from.Tables[0].Indexes = []schema.Index{{Name: "docs_lower_title", Concurrently: true, Parts: []schema.IndexPart{{Expr: "lower(title)"}}}}
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	to := docsSchema()
	to.Tables[0].Columns = to.Tables[0].Columns[:1]
	to.Tables[0].Indexes = nil
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, to, "drop_title", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil || m.Next == nil {
		t.Fatal(m, err)
	}
	head, tail := string(m.Files[m.Version+"_drop_title.up.sql"]), string(m.Files[m.Next.Version+"_"+m.Next.Name+".up.sql"])
	if !strings.Contains(head, `DROP COLUMN "title"`) || strings.Contains(head, "docs_lower_title") ||
		!strings.Contains(tail, "-- momo:no-transaction") || !strings.Contains(tail, `DROP INDEX CONCURRENTLY IF EXISTS "docs_lower_title"`) {
		t.Fatalf("want the column drop in the head and the idempotent concurrent drop in the tail\nhead:\n%s\ntail:\n%s", head, tail)
	}
}

// TestDiffNewPartitionedTableIndexIsNotConcurrent: the same for an index
// declared on a partitioned table that the diff creates.
func TestDiffNewPartitionedTableIndexIsNotConcurrent(t *testing.T) {
	d := docsSchema()
	d.Tables[0].Partition = &schema.Partition{Strategy: "range", Key: "RANGE (id)"}
	d.Tables[0].Indexes[0].Concurrently = true
	m, err := offlineEngine(t, fstest.MapFS{}, lockfreeClock).Diff(context.Background(), d, "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	up := string(m.Files[m.Version+"_init.up.sql"])
	if m.Next != nil || m.NoTx || strings.Contains(up, "CONCURRENTLY") || !strings.Contains(up, `CREATE INDEX "docs_title"`) {
		t.Fatalf("want a plain index in one transactional file:\n%s", up)
	}
}

// TestDiffAddCheckOnPartitionedTables: a check added to a partitioned and to a
// sub-partitioned table is NOT VALID in the head and validated in the tail.
func TestDiffAddCheckOnPartitionedTables(t *testing.T) {
	ctx := context.Background()
	mk := func(withCheck bool) *schema.Schema {
		s := docsSchema()
		s.Tables[0].Partition = &schema.Partition{Strategy: "range", Key: "RANGE (id)"}
		mid := schema.Table{
			Name:        "docs_mid",
			Columns:     s.Tables[0].Columns,
			PrimaryKey:  []string{"id"},
			PartitionOf: &schema.PartitionOf{Parent: "docs", Bound: "FOR VALUES FROM (0) TO (1000)"},
			Partition:   &schema.Partition{Strategy: "range", Key: "RANGE (id)"},
		}
		s.Tables = append(s.Tables, mid)
		if withCheck {
			s.Tables[0].Checks = []schema.Check{{Name: "docs_id_pos", Expr: "id > 0"}}
			s.Tables[1].Checks = []schema.Check{{Name: "docs_mid_id_pos", Expr: "id > 0"}}
		}
		return s
	}
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, mk(false), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, mk(true), "checks", migrate.DiffOptions{})
	if err != nil || m == nil || m.Next == nil {
		t.Fatal(m, err)
	}
	head, tail := string(m.Files[m.Version+"_checks.up.sql"]), string(m.Files[m.Next.Version+"_"+m.Next.Name+".up.sql"])
	for _, c := range []string{"docs_id_pos", "docs_mid_id_pos"} {
		if !strings.Contains(head, `ADD CONSTRAINT "`+c+`" CHECK (id > 0) NOT VALID`) || !strings.Contains(tail, `VALIDATE CONSTRAINT "`+c+`"`) {
			t.Fatalf("%s: want NOT VALID in the head and VALIDATE in the tail\nhead:\n%s\ntail:\n%s", c, head, tail)
		}
	}
}
