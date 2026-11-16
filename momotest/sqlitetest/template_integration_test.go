// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitetest_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/momotest/sqlitetest"
)

func count(t *testing.T, db *momo.DB, table string) int64 {
	t.Helper()
	rows, err := db.Driver().Query(t.Context(), "SELECT count(*) FROM "+table, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int64
	if !rows.Next() {
		t.Fatalf("count %s: no row (%v)", table, rows.Err())
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func itemsTemplate(builds *atomic.Int64) *sqlitetest.Template {
	return sqlitetest.NewTemplate(func(ctx context.Context, db *momo.DB) error {
		builds.Add(1)
		for _, stmt := range []string{
			"CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE)",
			"INSERT INTO items (name) VALUES ('seed')",
		} {
			if _, err := momo.Exec(ctx, db, stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

// TestTemplateIsBuiltOnceAndCopiesAreIsolated catches a Template that runs its
// build for every test (builds != 1), one whose copies share a file or write
// back to the template (the parallel inserts of the same unique name collide, or
// a later copy sees them), and one whose copy lacks the rows the build wrote.
func TestTemplateIsBuiltOnceAndCopiesAreIsolated(t *testing.T) {
	var builds atomic.Int64
	tpl := itemsTemplate(&builds)
	t.Run("group", func(t *testing.T) {
		for i := range 8 {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				db := tpl.New(t)
				if n := count(t, db, "items"); n != 1 {
					t.Fatalf("items = %d at the start, want the 1 seed row", n)
				}
				if _, err := momo.Exec(t.Context(), db, "INSERT INTO items (name) VALUES (?)", "same"); err != nil {
					t.Fatalf("insert: %v (another test's copy is shared)", err)
				}
				if n := count(t, db, "items"); n != 2 {
					t.Fatalf("items = %d, want 2", n)
				}
			})
		}
	})
	db := tpl.New(t)
	if n := count(t, db, "items"); n != 1 {
		t.Fatalf("items = %d after the parallel tests, want 1: a copy wrote back to the template", n)
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("build ran %d times, want 1", got)
	}
}

// TestTemplatesAreIndependent catches a package-level cache that hands every
// Template the first one's database: a changed build must give a changed
// schema.
func TestTemplatesAreIndependent(t *testing.T) {
	mk := func(table string) *sqlitetest.Template {
		return sqlitetest.NewTemplate(func(ctx context.Context, db *momo.DB) error {
			_, err := momo.Exec(ctx, db, "CREATE TABLE "+table+" (id INTEGER PRIMARY KEY)")
			return err
		})
	}
	a, b := mk("alpha"), mk("beta")
	dbA, dbB := a.New(t), b.New(t)
	if n := count(t, dbA, "alpha"); n != 0 {
		t.Fatalf("alpha = %d, want 0", n)
	}
	if n := count(t, dbB, "beta"); n != 0 {
		t.Fatalf("beta = %d, want 0", n)
	}
	if n := count(t, dbA, "sqlite_master WHERE name = 'beta'"); n != 0 {
		t.Fatal("template a has table beta: the second build leaked into the first")
	}
}
