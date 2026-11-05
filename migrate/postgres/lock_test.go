// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

func mustQueryRow(t *testing.T, c momo.PinnedConn, sql string, dest ...any) {
	t.Helper()
	rows, err := c.Query(context.Background(), sql, nil)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("query %q returned no row: %v", sql, rows.Err())
	}
	if err := rows.Scan(dest...); err != nil {
		t.Fatalf("scan %q: %v", sql, err)
	}
}

func TestLockHolderNamed(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	key := "momo:" + testdb.Schema(t) + ".momo_migrations" // same schema => same key => contention
	unlockA, _, err := mpg.New().Lock(ctx, pinned(t, db), key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlockA() }()
	_, _, err = mpg.New().Lock(ctx, pinned(t, db), key) // second connection, SAME schema
	if !errors.Is(err, migrate.ErrLocked) || !strings.Contains(err.Error(), "pid=") {
		t.Fatalf("want ErrLocked naming the holder pid, got %v", err)
	}
}

// Locks for different history schemas must not contend even though the advisory lock space is per database.
func TestLockKeyPerSchema(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	unlockA, _, err := mpg.New().Lock(ctx, pinned(t, db), "momo:"+testdb.Schema(t)+".momo_migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlockA() }()
	unlockB, _, err := mpg.New().Lock(ctx, pinned(t, db), "momo:"+testdb.Schema(t)+"_other.momo_migrations")
	if err != nil {
		t.Fatalf("a lock keyed by another schema must not be blocked: %v", err)
	}
	defer func() { _ = unlockB() }()
}

func TestLockTimeoutApplied(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	reset, err := mpg.New().Setup(ctx, c, migrate.SessionOptions{LockTimeout: 1500 * time.Millisecond, StatementTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var lt string
	mustQueryRow(t, c, `SHOW lock_timeout`, &lt)
	if lt != "1500ms" {
		t.Fatalf("lock_timeout = %q", lt)
	}
	var st string
	mustQueryRow(t, c, `SHOW statement_timeout`, &st)
	if st != "3s" {
		t.Fatalf("statement_timeout = %q", st)
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
}

// The lock must be released when the holder panics, so the next locker succeeds.
func TestLockReleasedAfterPanic(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	key := "momo:" + testdb.Schema(t) + ".panic.momo_migrations"
	func() {
		defer func() { _ = recover() }()
		unlock, _, err := mpg.New().Lock(ctx, pinned(t, db), key)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = unlock() }()
		panic("boom")
	}()
	unlock, _, err := mpg.New().Lock(ctx, pinned(t, db), key)
	if err != nil {
		t.Fatalf("lock must be free after a panic: %v", err)
	}
	_ = unlock()
}

// A cancelled context must not stop unlock from releasing the lock.
func TestUnlockAfterContextCancel(t *testing.T) {
	db := testopen.Postgres(t)
	key := "momo:" + testdb.Schema(t) + ".cancel.momo_migrations"
	ctx, cancel := context.WithCancel(context.Background())
	unlock, _, err := mpg.New().Lock(ctx, pinned(t, db), key)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := unlock(); err != nil {
		t.Fatalf("unlock with cancelled ctx: %v", err)
	}
	unlock2, _, err := mpg.New().Lock(context.Background(), pinned(t, db), key)
	if err != nil {
		t.Fatalf("lock must be free after unlock: %v", err)
	}
	_ = unlock2()
}

// Timeouts set by Setup must not leak to the next use of the pooled connection.
func TestSessionTimeoutsDoNotLeak(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	p := db.Driver().(momo.Pinner)
	c, err := p.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var before string
	mustQueryRow(t, c, `SHOW statement_timeout`, &before)
	reset, err := mpg.New().Setup(ctx, c, migrate.SessionOptions{LockTimeout: time.Second, StatementTimeout: 7 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	var lt, st string
	mustQueryRow(t, c, `SHOW lock_timeout`, &lt)
	mustQueryRow(t, c, `SHOW statement_timeout`, &st)
	if lt != "0" || st != before {
		t.Fatalf("after reset lock_timeout=%q statement_timeout=%q (before %q)", lt, st, before)
	}
	if err := c.Release(); err != nil {
		t.Fatal(err)
	}
}

// Two lockers: the second is refused with the holder named while the first holds, then succeeds after unlock.
func TestLockSecondWaitsForFirst(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	key := "momo:" + testdb.Schema(t) + ".two.momo_migrations"
	unlockA, _, err := mpg.New().Lock(ctx, pinned(t, db), key)
	if err != nil {
		t.Fatal(err)
	}
	_, holder, err := mpg.New().Lock(ctx, pinned(t, db), key)
	if !errors.Is(err, migrate.ErrLocked) {
		t.Fatalf("want ErrLocked, got %v", err)
	}
	if holder == nil {
		t.Fatal("holder func must be returned on contention")
	}
	text, herr := holder(ctx)
	if herr != nil || !strings.Contains(text, "held by pid=") {
		t.Fatalf("holder = %q, %v", text, herr)
	}
	if err := unlockA(); err != nil {
		t.Fatal(err)
	}
	unlockB, _, err := mpg.New().Lock(ctx, pinned(t, db), key)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	_ = unlockB()
}
