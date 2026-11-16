// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

// stagedDocs is the docs table, with the tenant column the staged change adds.
func stagedDocs(withTenant bool) *schema.Schema {
	cols := []schema.Column{
		{Name: "id", Type: "bigint"},
		{Name: "title", Type: "varchar(100)", Default: "''"},
	}
	if withTenant {
		cols = append(cols, schema.Column{Name: "tenant", Type: "bigint"})
	}
	return &schema.Schema{Dialect: "mysql", Tables: []schema.Table{{
		Name:       "docs",
		Columns:    cols,
		PrimaryKey: []string{"id"},
	}}}
}

func upSQL(m *migrate.Migration) string {
	for name, b := range m.Files {
		if strings.HasSuffix(name, ".up.sql") {
			return string(b)
		}
	}
	return ""
}

// TestMySQLStagedChange runs the staged NOT NULL flow through the engine on a
// populated table, on MySQL 8.4, MySQL 9.7 and MariaDB 11.8: add the column
// nullable, backfill, then make it NOT NULL. The rows survive and the next
// Diff is empty.
//
// Catches: a diff that emits NOT NULL directly for a populated table, which
// fails on the first row, a follow-up that never enforces or renders MODIFY
// COLUMN without the NOT NULL, and a snapshot that no longer matches the
// database after the flow.
func TestMySQLStagedChange(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		dir := t.TempDir()
		engineAt := func(day int) *migrate.Engine {
			when := time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
			return migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir), migrate.WithClock(func() time.Time { return when }))
		}
		count := func(q string) int64 {
			t.Helper()
			n, err := momo.Scalars[int64](ctx, db, q)
			if err != nil || len(n) != 1 {
				t.Fatalf("%s: %v, %v", q, n, err)
			}
			return n[0]
		}

		m1, err := engineAt(1).Diff(ctx, stagedDocs(false), "init", migrate.DiffOptions{})
		if err != nil || m1 == nil {
			t.Fatalf("init: %v, %v", m1, err)
		}
		writeMigration(t, dir, m1)
		if _, err := engineAt(1).Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		mustExec(t, db, "INSERT INTO docs (id) VALUES (1), (2), (3)")

		desired := stagedDocs(true)
		// A NOT NULL column without a default is refused outright.
		if _, err := engineAt(2).Diff(ctx, desired, "tenant", migrate.DiffOptions{}); !errors.Is(err, migrate.ErrStagedRequired) {
			t.Fatalf("a NOT NULL column without a default must need staging, got %v", err)
		}

		// Stage: the column is added nullable and applies to the populated table.
		m2, err := engineAt(2).Diff(ctx, desired, "tenant", migrate.DiffOptions{Stage: true})
		if err != nil || m2 == nil {
			t.Fatalf("stage: %v, %v", m2, err)
		}
		writeMigration(t, dir, m2)
		if _, err := engineAt(2).Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatalf("apply the staged add: %v", err)
		}
		if n := count("SELECT count(*) FROM docs WHERE tenant IS NULL"); n != 3 {
			t.Fatalf("the staged column is nullable and empty, NULL rows = %d", n)
		}

		// Backfill, then enforce.
		mustExec(t, db, "UPDATE docs SET tenant = id")
		m3, err := engineAt(3).Diff(ctx, desired, "not_null", migrate.DiffOptions{EnforceNotNull: true})
		if err != nil || m3 == nil {
			t.Fatalf("enforce: %v, %v", m3, err)
		}
		if up := strings.ToUpper(upSQL(m3)); !strings.Contains(up, "TENANT") || !strings.Contains(up, "NOT NULL") {
			t.Fatalf("the follow-up must make tenant NOT NULL:\n%s", upSQL(m3))
		}
		writeMigration(t, dir, m3)
		if _, err := engineAt(3).Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatalf("apply NOT NULL on the backfilled table: %v", err)
		}

		// The rows are intact and the constraint is enforced by the server.
		if n := count("SELECT count(*) FROM docs"); n != 3 {
			t.Fatalf("rows after the flow = %d, want 3", n)
		}
		if n := count("SELECT sum(tenant) FROM docs"); n != 6 {
			t.Fatalf("sum of the backfilled tenant = %d, want 6", n)
		}
		if n := count("SELECT count(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docs' AND column_name = 'tenant' AND is_nullable = 'NO'"); n != 1 {
			t.Fatalf("the column must be NOT NULL in the database, matching columns = %d", n)
		}
		if _, err := momo.Exec(ctx, db, "INSERT INTO docs (id) VALUES (9)"); err == nil {
			t.Fatal("an insert without tenant must now fail")
		}

		// Models and snapshot agree after the follow-up, and the database has not drifted.
		if again, err := engineAt(4).Diff(ctx, desired, "noop", migrate.DiffOptions{}); err != nil || again != nil {
			t.Fatalf("the diff after the follow-up must be empty: %v, %v", again, err)
		}
		drift, err := engineAt(4).Drift(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !drift.Clean() {
			t.Fatalf("drift after the staged flow: %+v", drift)
		}
	})
}

// concurrentVersions is the pending migrations both callers apply.
func concurrentVersions() fstest.MapFS {
	return fstest.MapFS{
		"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
		"20260101000001_b.up.sql": {Data: []byte("CREATE TABLE b (x int);")},
	}
}

// appliedVersions returns how many history rows each version has with the
// status applied.
func appliedVersions(t *testing.T, db *momo.DB) map[string]int {
	t.Helper()
	pc, err := db.Driver().(momo.Pinner).Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Release() }()
	rows, err := migrate.ReadHistory(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, r := range rows {
		if r.Status == migrate.StatusApplied {
			out[r.Version]++
		}
	}
	return out
}

func tableCount(t *testing.T, db *momo.DB, names ...string) int64 {
	t.Helper()
	q := "SELECT count(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('" + strings.Join(names, "', '") + "')"
	n, err := momo.Scalars[int64](context.Background(), db, q)
	if err != nil || len(n) != 1 {
		t.Fatalf("%s: %v, %v", q, n, err)
	}
	return n[0]
}

// TestMySQLConcurrentApply: two engines apply the same pending migrations on
// one database and one history table at the same time. The GET_LOCK lock lets
// one run at a time with no wait, so the other gets ErrLocked or finds nothing
// left to do, and each version is applied exactly once.
//
// Catches: a lock that does not exclude a second engine, which runs
// CREATE TABLE twice or records a version twice.
func TestMySQLConcurrentApply(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		fsys := concurrentVersions()
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e, err := migrate.NewEngine(db.Driver(), fsys, migrate.WithDialect(mmysql.New()), migrate.WithoutSum(), migrate.WithLockWait(0))
				if err != nil {
					errs <- err
					return
				}
				_, err = e.Apply(ctx, migrate.ApplyOptions{})
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		succeeded := 0
		for err := range errs {
			switch {
			case err == nil:
				succeeded++
			case !errors.Is(err, migrate.ErrLocked):
				t.Fatal(err)
			}
		}
		if succeeded == 0 {
			t.Fatal("neither caller applied: one must win the lock")
		}
		// The loser may have been locked out; the database is whole once a
		// later apply has run.
		e := migratetest.NewEngine(t, mmysql.New(), db.Driver(), fsys)
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		got := appliedVersions(t, db)
		if len(got) != 2 || got["20260101000000"] != 1 || got["20260101000001"] != 1 {
			t.Fatalf("each version must be applied exactly once, applied rows = %v", got)
		}
		if n := tableCount(t, db, "a", "b"); n != 2 {
			t.Fatalf("tables a and b present = %d, want 2", n)
		}
	})
}

// TestMySQLConcurrentApplyWaits: with a wait budget both callers succeed, the
// second finding nothing left to do, and the history is whole.
func TestMySQLConcurrentApplyWaits(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		fsys := concurrentVersions()
		var wg sync.WaitGroup
		reports := make(chan *migrate.Report, 2)
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e, err := migrate.NewEngine(db.Driver(), fsys, migrate.WithDialect(mmysql.New()), migrate.WithoutSum())
				if err != nil {
					errs <- err
					reports <- nil
					return
				}
				rep, err := e.Apply(ctx, migrate.ApplyOptions{LockWait: 20 * time.Second})
				reports <- rep
				errs <- err
			}()
		}
		wg.Wait()
		close(reports)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		total := 0
		for r := range reports {
			if r != nil {
				total += len(r.Applied)
			}
		}
		got := appliedVersions(t, db)
		if total != 2 || len(got) != 2 || got["20260101000000"] != 1 || got["20260101000001"] != 1 {
			t.Fatalf("2 versions must be applied once across 2 callers: reports applied %d, history %v", total, got)
		}
	})
}
