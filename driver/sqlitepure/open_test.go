// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitepure

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
)

func openPure(t *testing.T, cfg Config) (*sql.DB, momo.Driver) {
	t.Helper()
	db, d, err := Open(testdb.SQLitePath(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, d
}

func TestPureSQLiteCapabilitiesAndErrors(t *testing.T) {
	_, d := openPure(t, Config{})
	if d.(momo.CapabilityReporter).Capabilities().Vectors {
		t.Fatal("pure-Go driver must report Vectors=false")
	}
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE u (email text unique)", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, "INSERT INTO u VALUES ('a')", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, "INSERT INTO u VALUES ('a')", nil); !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("err = %v", err)
	}
}

func TestPureSQLiteConstraintKinds(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	for _, ddl := range []string{
		"CREATE TABLE p (id integer primary key, email text unique not null, n int check (n > 0))",
		"CREATE TABLE c (pid int references p(id))",
		"INSERT INTO p (id, email, n) VALUES (1, 'a@x', 1)",
	} {
		if _, err := d.Exec(ctx, ddl, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		sql  string
		want error
	}{
		{"unique", "INSERT INTO p (id, email, n) VALUES (2, 'a@x', 1)", momo.ErrUniqueViolation},
		{"primary key", "INSERT INTO p (id, email, n) VALUES (1, 'b@x', 1)", momo.ErrUniqueViolation},
		{"not null", "INSERT INTO p (id, email, n) VALUES (3, NULL, 1)", momo.ErrNotNullViolation},
		{"check", "INSERT INTO p (id, email, n) VALUES (4, 'c@x', 0)", momo.ErrCheckViolation},
		{"foreign key", "INSERT INTO c (pid) VALUES (99)", momo.ErrForeignKeyViolation},
	} {
		_, err := d.Exec(ctx, tc.sql, nil)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	_, err := d.Exec(ctx, "INSERT INTO p (id, email, n) VALUES (2, 'a@x', 1)", nil)
	var me *momo.Error
	if !errors.As(err, &me) || me.Table != "p" || me.Column != "email" {
		t.Fatalf("target = %+v", me)
	}
}

func TestPureSQLiteRecommendedConfig(t *testing.T) {
	db, _ := openPure(t, Config{})
	ctx := context.Background()
	var mode string
	var syncMode, fk, busy int
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncMode); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" || syncMode != 1 || fk != 1 || busy != 5000 {
		t.Fatalf("journal=%s synchronous=%d foreign_keys=%d busy_timeout=%d", mode, syncMode, fk, busy)
	}
}

func TestPureSQLiteBusyIsStructuredError(t *testing.T) {
	path := testdb.SQLitePath(t)
	cfg := Config{BusyTimeout: 50 * time.Millisecond}
	db1, d1, err := Open(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db1.Close() })
	db2, d2, err := Open(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })
	ctx := context.Background()
	if _, err := d1.Exec(ctx, "CREATE TABLE b (x int)", nil); err != nil {
		t.Fatal(err)
	}
	tx, err := d1.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "INSERT INTO b VALUES (1)", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d2.Exec(ctx, "INSERT INTO b VALUES (2)", nil); !errors.Is(err, momo.ErrBusy) {
		t.Fatalf("second writer err = %v, want momo.ErrBusy", err)
	}
}

func TestPureSQLiteIsolationAndReadOnly(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	if _, err := d.Begin(ctx, momo.TxOptions{Isolation: sql.LevelReadUncommitted}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("read uncommitted err = %v, want ErrUnsupported", err)
	}
	if _, err := d.Exec(ctx, "CREATE TABLE ro (x int)", nil); err != nil {
		t.Fatal(err)
	}
	tx, err := d.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO ro VALUES (1)", nil); err == nil {
		t.Fatal("write inside a read-only transaction succeeded")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("second rollback err = %v, want ErrTxDone", err)
	}
	// query_only must be off again for the next user of the connection.
	if _, err := d.Exec(ctx, "INSERT INTO ro VALUES (2)", nil); err != nil {
		t.Fatalf("write after a read-only transaction: %v", err)
	}
}

func TestPureSQLiteCommitAndRollback(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE tx (x int)", nil); err != nil {
		t.Fatal(err)
	}
	tx, err := d.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO tx VALUES (1)", nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = d.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO tx VALUES (2)", nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Query(ctx, "SELECT x FROM tx", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var x int
		if err := rows.Scan(&x); err != nil {
			t.Fatal(err)
		}
		got = append(got, x)
	}
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("rows = %v, want [2]", got)
	}
}

func TestPureSQLitePinnedAndStats(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 1 {
		t.Fatalf("AcquiredConns = %d", st.AcquiredConns)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "SELECT 1", nil); err == nil {
		t.Fatal("pinned connection usable after Release")
	}
}

func TestPureSQLiteColumnsAndTimes(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE t (id integer primary key, at text, opt text)", nil); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 8, 10, 11, 12, 123000000, time.UTC)
	if _, err := d.Exec(ctx, "INSERT INTO t (at, opt) VALUES (?, NULL)", []any{at}); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Query(ctx, "SELECT id AS ident, at, opt FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.(momo.ColumnNamer).Columns()
	if err != nil || strings.Join(cols, ",") != "ident,at,opt" {
		t.Fatalf("columns = %v, err = %v", cols, err)
	}
	if !rows.Next() {
		t.Fatal("no row")
	}
	var id int
	var got time.Time
	var opt *time.Time
	if err := rows.Scan(&id, &got, &opt); err != nil {
		t.Fatal(err)
	}
	if !got.Equal(at) || opt != nil {
		t.Fatalf("at = %v opt = %v", got, opt)
	}
}

func TestPureSQLiteScanErrorHidesValue(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	rows, err := d.Query(ctx, "SELECT 'secret-value'", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no row")
	}
	var n int
	err = rows.Scan(&n)
	if err == nil {
		t.Fatal("scanning text into int succeeded")
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("the error echoes the stored value: %v", err)
	}
}

func TestPureSQLiteInMemoryAndDSNRefusals(t *testing.T) {
	db, _, err := Open(":memory:", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(":memory:", Config{MaxOpenConns: 2}); err == nil {
		t.Fatal("two connections to a private in-memory database accepted")
	}
	if _, _, err := Open(testdb.SQLitePath(t)+"?_pragma=foreign_keys(0)", Config{}); err == nil {
		t.Fatal("a _pragma DSN parameter was accepted")
	}
}
