// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestFromSQLDBPinsAndCounts(t *testing.T) {
	db, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	d := FromSQLDB(db)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 1 {
		t.Fatalf("AcquiredConns = %d while pinned, want 1", st.AcquiredConns)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestFromConnConfigExecMode(t *testing.T) {
	cfg, err := pgx.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	db, drv := FromConnConfig(cfg)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	rows, err := drv.Query(ctx, "SELECT $1::int", []any{1})
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var x int
	if err := conn.QueryRowContext(ctx, "SELECT $1::int", 1).Scan(&x); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM pg_prepared_statements").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("exec mode left %d named statements on the session", n)
	}
}

func openSQLDriver(t *testing.T) (*sql.DB, momo.Driver) {
	t.Helper()
	db, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, FromSQLDB(db)
}

func TestFromSQLDBTxOptionsAndDone(t *testing.T) {
	_, d := openSQLDriver(t)
	ctx := context.Background()
	tx, err := d.Begin(ctx, momo.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, "SHOW transaction_read_only", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ro string
	if !rows.Next() {
		t.Fatalf("no row: %v", rows.Err())
	}
	if err := rows.Scan(&ro); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if ro != "on" {
		t.Fatalf("transaction_read_only = %q, want on", ro)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("second Commit err = %v, want ErrTxDone", err)
	}
}

func TestFromSQLDBClassifiesErrors(t *testing.T) {
	_, d := openSQLDriver(t)
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE sqldb_uniq (id int PRIMARY KEY)", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, "INSERT INTO sqldb_uniq VALUES (1)", nil); err != nil {
		t.Fatal(err)
	}
	_, err := d.Exec(ctx, "INSERT INTO sqldb_uniq VALUES (1)", nil)
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("err = %v, want ErrUniqueViolation", err)
	}
}

func TestFromSQLDBPinReleaseClosesRows(t *testing.T) {
	db, d := openSQLDriver(t)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := p.Query(ctx, "SELECT generate_series(1, 3)", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Release(); err != nil {
		t.Fatalf("Release with open rows: %v", err)
	}
	if err := p.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
	if rows.Next() {
		t.Fatal("rows still usable after Release")
	}
	if _, err := p.Exec(ctx, "SELECT 1", nil); err == nil {
		t.Fatal("Exec after Release succeeded")
	}
	if st := db.Stats(); st.InUse != 0 {
		t.Fatalf("InUse = %d after Release, want 0", st.InUse)
	}
}

func TestFromSQLDBDiscardDestroysSession(t *testing.T) {
	db, d := openSQLDriver(t)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var pid1 int
	scanOne(t, p, "SELECT pg_backend_pid()", &pid1)
	if err := p.(momo.Discarder).Discard(); err != nil {
		t.Fatal(err)
	}
	if err := p.Release(); err != nil {
		t.Fatalf("Release after Discard: %v", err)
	}
	if st := db.Stats(); st.OpenConnections != 0 {
		t.Fatalf("OpenConnections = %d after Discard, want 0", st.OpenConnections)
	}
	var pid2 int
	if err := db.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid2); err != nil {
		t.Fatal(err)
	}
	if pid1 == pid2 {
		t.Fatalf("session %d was reused after Discard", pid1)
	}
}
