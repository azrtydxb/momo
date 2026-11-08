// SPDX-License-Identifier: Apache-2.0

//go:build integration

package testopen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEachRunsBothDialects(t *testing.T) {
	testdb.PostgresURL(t) // skip or fail through testdb before counting subtests
	var ran []dialect.Name
	Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ran = append(ran, d)
		if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
	})
	if len(ran) != 2 || ran[0] != dialect.Postgres || ran[1] != dialect.SQLite {
		t.Fatalf("Each ran %v, want [postgres sqlite]", ran)
	}
}

func TestEachNamesSubtestAfterDialect(t *testing.T) {
	Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !strings.HasSuffix(t.Name(), "/"+string(d)) {
			t.Fatalf("subtest %q is not named after dialect %q", t.Name(), d)
		}
		if db.Dialect().Name != d {
			t.Fatalf("db dialect %q, subtest dialect %q", db.Dialect().Name, d)
		}
	})
}

// countHook counts statements reaching the hook.
type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {}

func TestOptionsReachSQLite(t *testing.T) {
	hook := &countHook{}
	db := SQLite(t, momo.WithHook(hook))
	if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if hook.n != 1 {
		t.Fatalf("WithHook passed to SQLite did not reach the database: %d events", hook.n)
	}
}

func TestOptionsReachDB(t *testing.T) {
	hook := &countHook{}
	db := Postgres(t, momo.WithHook(hook))
	if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if hook.n != 1 {
		t.Fatalf("WithHook passed to Postgres did not reach the database: %d events", hook.n)
	}
}

func TestEachPassesOptions(t *testing.T) {
	testdb.PostgresURL(t) // skip or fail through testdb before counting subtests
	hook := &countHook{}
	ran := 0
	Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ran++
		if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
	}, momo.WithHook(hook))
	if ran != len(openers) || hook.n != len(openers) {
		t.Fatalf("ran %d dialects, hook saw %d statements, want %d of each", ran, hook.n, len(openers))
	}
}

func currentSchema(t *testing.T, db *momo.DB) string {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), "SELECT current_schema()", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var s string
	if !rows.Next() {
		t.Fatalf("no row: %v", rows.Err())
	}
	if err := rows.Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEachSubtestsAreIsolated(t *testing.T) {
	testdb.PostgresURL(t) // skip or fail through testdb before counting subtests
	var schemas []string
	var dbs []*momo.DB
	for range 2 {
		Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			dbs = append(dbs, db)
			if d == dialect.Postgres {
				schemas = append(schemas, currentSchema(t, db))
			}
			if _, err := momo.Exec(context.Background(), db, "CREATE TABLE iso (id int)"); err != nil {
				t.Fatalf("a table from another subtest leaked into this schema: %v", err)
			}
		})
	}
	if len(schemas) != 2 || schemas[0] == schemas[1] || !strings.HasPrefix(schemas[0], "t_") {
		t.Fatalf("subtests shared a schema: %v", schemas)
	}
	// The subtests are over, so t.Cleanup has closed their pools.
	for _, db := range dbs {
		if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err == nil {
			t.Fatal("database still usable after its subtest finished: pool not closed")
		}
	}
}

// TestRequireFailsWithoutURL runs the Each test in a child process with the
// URL unset and MOMO_REQUIRE_INTEGRATION=1: it must fail, not skip.
func TestRequireFailsWithoutURL(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestEachRunsBothDialects$", "-test.v")
	cmd.Env = append(os.Environ(), "MOMO_REQUIRE_INTEGRATION=1", "MOMO_TEST_POSTGRES_URL=", "MOMO_INTEGRATION_DIALECTS=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child passed or skipped, want failure:\n%s", out)
	}
	if strings.Contains(string(out), "SKIP") {
		t.Fatalf("child skipped:\n%s", out)
	}
}

// poolWaitHook keeps the largest PoolWait it sees.
type poolWaitHook struct{ maxNS atomic.Int64 }

func (h *poolWaitHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	return ctx
}

func (h *poolWaitHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	for {
		cur := h.maxNS.Load()
		if int64(e.PoolWait) <= cur || h.maxNS.CompareAndSwap(cur, int64(e.PoolWait)) {
			return
		}
	}
}

// TestEachPoolRecordsPoolWait proves the pool testopen builds is instrumented:
// with every connection pinned, a statement that gives up waiting still
// reports how long it waited.
func TestEachPoolRecordsPoolWait(t *testing.T) {
	testdb.PostgresURL(t) // skip or fail through testdb before asserting on the hook
	hook := &poolWaitHook{}
	Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("pool wait is recorded by the pgx pool instrumentation only")
		}
		pinner, ok := db.Driver().(momo.Pinner)
		if !ok {
			t.Skip("driver cannot pin connections")
		}
		var held []momo.PinnedConn
		t.Cleanup(func() {
			for _, pc := range held {
				_ = pc.Release()
			}
		})
		for { // pin until the pool is exhausted
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			pc, err := pinner.Pin(ctx)
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			if err != nil {
				t.Fatalf("pin: %v", err)
			}
			held = append(held, pc)
			if len(held) > 1024 {
				t.Fatal("pool never exhausted")
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		if _, err := momo.Exec(ctx, db, "SELECT 1"); err == nil {
			t.Fatal("want an error from the exhausted pool")
		}
	}, momo.WithHook(hook))
	if got := time.Duration(hook.maxNS.Load()); got < 50*time.Millisecond {
		t.Fatalf("PoolWait = %v, want about 80ms: testopen's pool is not instrumented", got)
	}
}

// TestClosePoolReportsLeak proves a leaked connection is reported instead of
// hanging pool.Close forever.
func TestClosePoolReportsLeak(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var msg string
	start := time.Now()
	closePool(pool, 100*time.Millisecond, func(format string, args ...any) {
		msg = fmt.Sprintf(format, args...)
	})
	if time.Since(start) > 3*time.Second {
		t.Fatal("closePool did not give up on a leaked connection")
	}
	if !strings.Contains(msg, "1 connection") {
		t.Fatalf("leak report = %q, want the acquired-connection count", msg)
	}
	conn.Release() // lets the background Close finish
}

func TestClosePoolQuietWhenClean(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	closePool(pool, 5*time.Second, func(format string, args ...any) {
		t.Errorf("unexpected report: "+format, args...)
	})
}
