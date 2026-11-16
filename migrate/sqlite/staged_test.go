// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func stagedDocsSchema() *schema.Schema {
	return &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{{
		Name:       "docs",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "title", Type: "TEXT", Default: "''"}},
		PrimaryKey: []string{"id"},
		Indexes:    []schema.Index{{Name: "docs_title", Parts: []schema.IndexPart{{Column: "title"}}}},
	}}}
}

// queryString runs q on a connection that is released before it returns and
// returns the first column of the first row.
func queryString(t *testing.T, db *momo.DB, q string) string {
	t.Helper()
	c := pinned(t, db)
	var s string
	scanOne(t, c, q, &s)
	release(t, c)
	return s
}

// TestSQLiteStagedChange runs the staged NOT NULL flow on a populated table
// through the engine: the column is added nullable (a native ALTER), backfilled
// with a data statement, then made NOT NULL, which SQLite can only do with a
// table rebuild; the rebuild applies because every row satisfies it.
//
// Catches: a diff that emits NOT NULL directly for a populated table without a
// default, a rebuild that loses rows or the index, and a follow-up that never
// enforces the constraint.
func TestSQLiteStagedChange(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	eng := func(files map[string][]byte, ts string) *migrate.Engine {
		return newEngine(t, db, migratetest.FS(files), ts)
	}

	m1, err := eng(nil, "20260101000000").Diff(ctx, stagedDocsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatalf("init: %v %v", m1, err)
	}
	if _, err := eng(m1.Files, "20260101000000").Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db.Driver(), `INSERT INTO docs (id) VALUES (1), (2), (3)`)

	desired := stagedDocsSchema()
	desired.Tables[0].Columns = append(desired.Tables[0].Columns, schema.Column{Name: "tenant", Type: "INTEGER"})

	// A NOT NULL column without a default is refused outright.
	if _, err := eng(m1.Files, "20260102000000").Diff(ctx, desired, "tenant", migrate.DiffOptions{}); !errors.Is(err, migrate.ErrStagedRequired) {
		t.Fatalf("a NOT NULL column without a default must need staging, got %v", err)
	}

	// Stage: the column is added nullable and applies to the populated table.
	m2, err := eng(m1.Files, "20260102000000").Diff(ctx, desired, "tenant", migrate.DiffOptions{Stage: true})
	if err != nil || m2 == nil {
		t.Fatalf("stage: %v %v", m2, err)
	}
	staged := mergeFiles(m1.Files, m2.Files)
	if _, err := eng(staged, "20260102000000").Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply the staged add: %v", err)
	}
	if n := queryString(t, db, `SELECT count(*) FROM docs WHERE tenant IS NULL`); n != "3" {
		t.Fatalf("the staged column is nullable and empty, NULL rows = %s", n)
	}

	// Backfill, then enforce.
	mustExec(t, db.Driver(), `UPDATE docs SET tenant = id`)
	m3, err := eng(staged, "20260103000000").Diff(ctx, desired, "not_null", migrate.DiffOptions{EnforceNotNull: true})
	if err != nil || m3 == nil {
		t.Fatalf("enforce: %v %v", m3, err)
	}
	if up := upFile(t, m3.Files); !strings.Contains(up, `CREATE TABLE "new_docs"`) || !strings.Contains(up, `"tenant" INTEGER NOT NULL`) {
		t.Fatalf("the follow-up must rebuild docs with tenant NOT NULL:\n%s", up)
	}
	enforced := mergeFiles(staged, m3.Files)
	if _, err := eng(enforced, "20260103000000").Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply the rebuild on the backfilled table: %v", err)
	}

	if n := queryString(t, db, `SELECT count(*) FROM docs WHERE tenant = id`); n != "3" {
		t.Fatalf("the rebuild must keep every row with its backfilled value, rows = %s", n)
	}
	if v := queryString(t, db, `SELECT "notnull" FROM pragma_table_info('docs') WHERE name = 'tenant'`); v != "1" {
		t.Fatalf("the column must be NOT NULL in the database, notnull = %s", v)
	}
	if n := queryString(t, db, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'docs_title'`); n != "1" {
		t.Fatalf("the rebuild must recreate the index, found %s", n)
	}
	if _, err := db.Driver().Exec(ctx, `INSERT INTO docs (id) VALUES (9)`, nil); err == nil {
		t.Fatal("an insert without tenant must now fail")
	}
	if again, err := eng(enforced, "20260104000000").Diff(ctx, desired, "noop", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("models and snapshot agree after the follow-up: %v %v", again, err)
	}
}

// TestSQLiteConcurrentApply: two engines on one SQLite file and one history
// table; the lock lets only one run at a time and each version is applied
// exactly once. A run that loses the race may report ErrLocked and nothing else.
//
// Catches: a lock that lets both runs in (a version applied twice or a
// "table already exists" failure), and a loser that fails with a raw busy
// error instead of the lock error.
func TestSQLiteConcurrentApply(t *testing.T) {
	db := openSQLite(t, 2)
	ctx := context.Background()
	fsys := fstest.MapFS{
		"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x integer);")},
		"20260101000001_b.up.sql": {Data: []byte("CREATE TABLE b (x integer);")},
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, err := migrate.NewEngine(db.Driver(), fsys, migrate.WithDialect(msqlite.New()), migrate.WithoutSum(), migrate.WithLockWait(0))
			if err != nil {
				errs <- err
				return
			}
			<-start
			_, err = e.Apply(ctx, migrate.ApplyOptions{})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	succeeded := 0
	for err := range errs {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, migrate.ErrLocked) {
			t.Fatal(err)
		}
	}
	if succeeded == 0 {
		t.Fatal("both runs reported ErrLocked; one of them must have applied the migrations")
	}

	c := pinned(t, db)
	rows, err := migrate.ReadHistory(ctx, c)
	release(t, c)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, r := range rows {
		if r.Status == migrate.StatusApplied {
			seen[r.Version]++
		}
	}
	if len(seen) != 2 || seen["20260101000000"] != 1 || seen["20260101000001"] != 1 {
		t.Fatalf("each version must be applied exactly once, applied rows = %v (history %+v)", seen, rows)
	}
}
