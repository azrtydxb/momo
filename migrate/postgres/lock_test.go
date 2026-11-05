// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
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

// oneConnDB opens a pool limited to one connection, so a re-pin gets the same backend back.
func oneConnDB(t *testing.T) (*momo.DB, momo.PoolStater) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	drv := pgxdriver.New(pool)
	return momo.New(drv), drv.(momo.PoolStater)
}

func pin(t *testing.T, db *momo.DB) momo.PinnedConn {
	t.Helper()
	c, err := db.Driver().(momo.Pinner).Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Timeouts set by Setup must not leak to the next use of the pooled connection.
// With MaxConns=1 the re-pinned connection is the same backend.
func TestSessionTimeoutsDoNotLeak(t *testing.T) {
	db, _ := oneConnDB(t)
	ctx := context.Background()
	c := pin(t, db)
	var pid1 int32
	mustQueryRow(t, c, `SELECT pg_backend_pid()`, &pid1)
	var beforeLT, beforeST string
	mustQueryRow(t, c, `SHOW lock_timeout`, &beforeLT)
	mustQueryRow(t, c, `SHOW statement_timeout`, &beforeST)
	reset, err := mpg.New().Setup(ctx, c, migrate.SessionOptions{LockTimeout: time.Second, StatementTimeout: 7 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var during string
	mustQueryRow(t, c, `SHOW statement_timeout`, &during)
	if during != "7s" {
		t.Fatalf("statement_timeout during apply = %q", during)
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	if err := c.Release(); err != nil {
		t.Fatal(err)
	}
	c2 := pin(t, db)
	defer func() { _ = c2.Release() }()
	var pid2 int32
	var lt, st string
	mustQueryRow(t, c2, `SELECT pg_backend_pid()`, &pid2)
	mustQueryRow(t, c2, `SHOW lock_timeout`, &lt)
	mustQueryRow(t, c2, `SHOW statement_timeout`, &st)
	if pid2 != pid1 {
		t.Fatalf("re-pin returned another backend (%d, want %d); the test would prove nothing", pid2, pid1)
	}
	if lt != beforeLT || st != beforeST {
		t.Fatalf("timeouts leaked: lock_timeout=%q statement_timeout=%q (defaults %q, %q)", lt, st, beforeLT, beforeST)
	}
}

// failingConn fails statements containing failOn, to force the unlock and reset error paths.
type failingConn struct {
	momo.PinnedConn
	failOn    string
	discarded bool
}

func (f *failingConn) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	if strings.Contains(sql, f.failOn) {
		return 0, errors.New("forced failure")
	}
	return f.PinnedConn.Exec(ctx, sql, args)
}

func (f *failingConn) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if strings.Contains(sql, f.failOn) {
		return nil, errors.New("forced failure")
	}
	return f.PinnedConn.Query(ctx, sql, args)
}

func (f *failingConn) Discard() error {
	f.discarded = true
	return f.PinnedConn.(momo.Discarder).Discard()
}

func waitPool(t *testing.T, ps momo.PoolStater, ok func(momo.PoolStats) bool) momo.PoolStats {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := ps.PoolStats()
		if ok(st) || time.Now().After(deadline) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A failed RESET must discard the connection: the pool destroys the backend and a fresh pin has default timeouts.
func TestResetFailureDiscardsConn(t *testing.T) {
	db, ps := oneConnDB(t)
	ctx := context.Background()
	real := pin(t, db)
	var pid1 int32
	mustQueryRow(t, real, `SELECT pg_backend_pid()`, &pid1)
	var defST string
	mustQueryRow(t, real, `SHOW statement_timeout`, &defST)
	c := &failingConn{PinnedConn: real, failOn: "RESET"}
	reset, err := mpg.New().Setup(ctx, c, migrate.SessionOptions{LockTimeout: time.Second, StatementTimeout: 7 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := reset(); err == nil {
		t.Fatal("reset must report the failure")
	}
	if !c.discarded {
		t.Fatal("a failed reset must discard the connection")
	}
	st := waitPool(t, ps, func(s momo.PoolStats) bool { return s.AcquiredConns == 0 && s.TotalConns == 0 })
	if st.AcquiredConns != 0 || st.TotalConns != 0 {
		t.Fatalf("pool stats after discard = %+v, want no connections", st)
	}
	fresh := pin(t, db)
	defer func() { _ = fresh.Release() }()
	var pid2 int32
	var lt, stout string
	mustQueryRow(t, fresh, `SELECT pg_backend_pid()`, &pid2)
	mustQueryRow(t, fresh, `SHOW lock_timeout`, &lt)
	mustQueryRow(t, fresh, `SHOW statement_timeout`, &stout)
	if pid2 == pid1 {
		t.Fatal("the discarded backend was reused")
	}
	if lt != "0" || stout != defST {
		t.Fatalf("fresh conn has lock_timeout=%q statement_timeout=%q, want defaults", lt, stout)
	}
}

// A failed unlock must discard the connection so the advisory lock dies with the session.
func TestUnlockFailureDiscardsConn(t *testing.T) {
	db, ps := oneConnDB(t)
	ctx := context.Background()
	key := "momo:" + testdb.Schema(t) + ".unlockfail.momo_migrations"
	c := &failingConn{PinnedConn: pin(t, db), failOn: "pg_advisory_unlock"}
	unlock, _, err := mpg.New().Lock(ctx, c, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err == nil {
		t.Fatal("unlock must report the failure")
	}
	if !c.discarded {
		t.Fatal("a failed unlock must discard the connection")
	}
	st := waitPool(t, ps, func(s momo.PoolStats) bool { return s.AcquiredConns == 0 && s.TotalConns == 0 })
	if st.AcquiredConns != 0 || st.TotalConns != 0 {
		t.Fatalf("pool stats after discard = %+v, want no connections", st)
	}
	fresh := pin(t, db)
	defer func() { _ = fresh.Release() }()
	unlock2, _, err := mpg.New().Lock(ctx, fresh, key)
	if err != nil {
		t.Fatalf("the lock must be free after the holder session was discarded: %v", err)
	}
	_ = unlock2()
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
