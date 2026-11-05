// SPDX-License-Identifier: Apache-2.0

package testopen

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
)

func TestEachRunsPostgres(t *testing.T) {
	testdb.PostgresURL(t) // skip or fail through testdb before counting subtests
	var ran []dialect.Name
	Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ran = append(ran, d)
		rows, err := db.Driver().Query(context.Background(), "SELECT 1", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	})
	if len(ran) != len(openers) {
		t.Fatalf("Each ran %d dialects, registered %d", len(ran), len(openers))
	}
	if len(ran) == 0 || ran[0] != dialect.Postgres {
		t.Fatalf("Each ran %v, want postgres first", ran)
	}
}

// countHook counts statements reaching the hook.
type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {}

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
			schemas = append(schemas, currentSchema(t, db))
			dbs = append(dbs, db)
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
	cmd := exec.Command(os.Args[0], "-test.run=^TestEachRunsPostgres$", "-test.v")
	cmd.Env = append(os.Environ(), "MOMO_REQUIRE_INTEGRATION=1", "MOMO_TEST_POSTGRES_URL=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child passed or skipped, want failure:\n%s", out)
	}
	if strings.Contains(string(out), "SKIP") {
		t.Fatalf("child skipped:\n%s", out)
	}
}
