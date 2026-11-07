// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/vector"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func adoptTable(t *testing.T, db momo.Querier) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), db, "CREATE TABLE adopt_t (id int PRIMARY KEY, name text)"); err != nil {
		t.Fatal(err)
	}
}

func adoptSQLDB(t *testing.T) (*sql.DB, *momo.DB) {
	t.Helper()
	sdb, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	mdb := momo.New(FromSQLDB(sdb))
	adoptTable(t, mdb)
	return sdb, mdb
}

func adoptPgxDB(t *testing.T) (*pgxpool.Pool, *momo.DB) {
	t.Helper()
	pool := openPool(t)
	mdb := momo.New(New(pool))
	adoptTable(t, mdb)
	return pool, mdb
}

func countRows(t *testing.T, q momo.Querier) int {
	t.Helper()
	v, err := momo.Scalars[int](context.Background(), q, "SELECT count(*)::int FROM adopt_t")
	if err != nil || len(v) != 1 {
		t.Fatalf("count: %v %v", v, err)
	}
	return v[0]
}

func TestAdoptSQLTxMixedLegacyAndMomo(t *testing.T) {
	ctx := context.Background()
	sdb, mdb := adoptSQLDB(t)

	// Caller commits.
	stx, err := sdb.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stx.Rollback() }()
	if _, err := stx.ExecContext(ctx, "INSERT INTO adopt_t VALUES (1, 'legacy')"); err != nil {
		t.Fatal(err)
	}
	mtx := mdb.AdoptTx(SQLTx(stx))
	if _, err := momo.Exec(ctx, mtx, "INSERT INTO adopt_t VALUES (?, ?)", 2, "momo"); err != nil {
		t.Fatal(err)
	}
	// Legacy helper style: SELECT ... FOR UPDATE on the same *sql.Tx after momo ran.
	var name string
	if err := stx.QueryRowContext(ctx, "SELECT name FROM adopt_t WHERE id = 2 FOR UPDATE").Scan(&name); err != nil || name != "momo" {
		t.Fatalf("legacy read after momo = %q, %v", name, err)
	}
	// And momo can lock a row legacy inserted.
	if got, err := momo.Scalars[string](ctx, mtx, "SELECT name FROM adopt_t WHERE id = 1 FOR UPDATE"); err != nil || len(got) != 1 || got[0] != "legacy" {
		t.Fatalf("momo FOR UPDATE = %v, %v", got, err)
	}
	if err := mtx.Commit(ctx); !errors.Is(err, momo.ErrAdoptedTx) {
		t.Fatalf("Commit = %v, want ErrAdoptedTx", err)
	}
	if got, ok := UnwrapSQLTx(mtx); !ok || got != stx {
		t.Fatal("UnwrapSQLTx did not return the adopted *sql.Tx")
	}
	if err := stx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, mdb); n != 2 {
		t.Fatalf("rows after caller commit = %d, want 2", n)
	}
	// After the caller finished it, momo statements report a finished transaction.
	if _, err := momo.Exec(ctx, mtx, "SELECT 1"); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("statement after caller commit = %v, want ErrTxDone", err)
	}

	// Caller rolls back.
	stx2, err := sdb.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	mtx2 := mdb.AdoptTx(SQLTx(stx2))
	if _, err := momo.Exec(ctx, mtx2, "INSERT INTO adopt_t VALUES (?, ?)", 3, "gone"); err != nil {
		t.Fatal(err)
	}
	if err := mtx2.Rollback(ctx); !errors.Is(err, momo.ErrAdoptedTx) {
		t.Fatalf("Rollback = %v, want ErrAdoptedTx", err)
	}
	if err := stx2.Rollback(); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, mdb); n != 2 {
		t.Fatalf("rows after caller rollback = %d, want 2", n)
	}
}

func TestAdoptSQLTxClassifiesErrorsAndSavepoints(t *testing.T) {
	ctx := context.Background()
	sdb, mdb := adoptSQLDB(t)
	stx, err := sdb.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stx.Rollback() }()
	mtx := mdb.AdoptTx(SQLTx(stx))
	if _, err := momo.Exec(ctx, mtx, "INSERT INTO adopt_t VALUES (1, 'a')"); err != nil {
		t.Fatal(err)
	}
	// The unique violation poisons only the savepoint; the outer tx survives.
	err = mtx.Tx(ctx, func(in *momo.Tx) error {
		_, err := momo.Exec(ctx, in, "INSERT INTO adopt_t VALUES (1, 'dup')")
		return err
	})
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("err = %v, want ErrUniqueViolation", err)
	}
	if got, ok := UnwrapSQLTx(mtx); !ok || got != stx {
		t.Fatal("unwrap failed")
	}
	var n int
	if err := stx.QueryRowContext(ctx, "SELECT count(*) FROM adopt_t").Scan(&n); err != nil || n != 1 {
		t.Fatalf("legacy count after savepoint rollback = %d, %v", n, err)
	}
	// Inside a savepoint scope, unwrap yields the outer transaction.
	_ = mtx.Tx(ctx, func(in *momo.Tx) error {
		if got, ok := UnwrapSQLTx(in); !ok || got != stx {
			t.Error("savepoint scope must unwrap to the outer *sql.Tx")
		}
		return nil
	})
}

func TestAdoptPgxTxMixedLegacyAndMomo(t *testing.T) {
	ctx := context.Background()
	pool, mdb := adoptPgxDB(t)

	ptx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptx.Rollback(ctx) }()
	if _, err := ptx.Exec(ctx, "INSERT INTO adopt_t VALUES (1, 'legacy')"); err != nil {
		t.Fatal(err)
	}
	mtx := mdb.AdoptTx(PgxTx(ptx))
	if _, err := momo.Exec(ctx, mtx, "INSERT INTO adopt_t VALUES (?, ?)", 2, "momo"); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := ptx.QueryRow(ctx, "SELECT name FROM adopt_t WHERE id = 2 FOR UPDATE").Scan(&name); err != nil || name != "momo" {
		t.Fatalf("legacy read after momo = %q, %v", name, err)
	}
	if got, err := momo.Scalars[string](ctx, mtx, "SELECT name FROM adopt_t WHERE id = 1 FOR UPDATE"); err != nil || len(got) != 1 || got[0] != "legacy" {
		t.Fatalf("momo FOR UPDATE = %v, %v", got, err)
	}
	if err := mtx.Commit(ctx); !errors.Is(err, momo.ErrAdoptedTx) {
		t.Fatalf("Commit = %v, want ErrAdoptedTx", err)
	}
	if got, ok := UnwrapPgxTx(mtx); !ok || got != pgx.Tx(ptx) {
		t.Fatal("UnwrapPgxTx did not return the adopted pgx.Tx")
	}
	if _, ok := UnwrapSQLTx(mtx); ok {
		t.Fatal("UnwrapSQLTx must be false for the pgx driver")
	}
	if err := ptx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, mdb); n != 2 {
		t.Fatalf("rows after caller commit = %d, want 2", n)
	}
	if _, err := momo.Exec(ctx, mtx, "SELECT 1"); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("statement after caller commit = %v, want ErrTxDone", err)
	}

	ptx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mtx2 := mdb.AdoptTx(PgxTx(ptx2))
	if _, err := momo.Exec(ctx, mtx2, "INSERT INTO adopt_t VALUES (?, ?)", 3, "gone"); err != nil {
		t.Fatal(err)
	}
	if err := mtx2.Rollback(ctx); !errors.Is(err, momo.ErrAdoptedTx) {
		t.Fatalf("Rollback = %v, want ErrAdoptedTx", err)
	}
	if err := ptx2.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, mdb); n != 2 {
		t.Fatalf("rows after caller rollback = %d, want 2", n)
	}
}

func TestAdoptPgxTxClassifiesErrorsAndSavepoints(t *testing.T) {
	ctx := context.Background()
	pool, mdb := adoptPgxDB(t)
	ptx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptx.Rollback(ctx) }()
	mtx := mdb.AdoptTx(PgxTx(ptx))
	if _, err := momo.Exec(ctx, mtx, "INSERT INTO adopt_t VALUES (1, 'a')"); err != nil {
		t.Fatal(err)
	}
	err = mtx.Tx(ctx, func(in *momo.Tx) error {
		if got, ok := UnwrapPgxTx(in); !ok || got != pgx.Tx(ptx) {
			t.Error("savepoint scope must unwrap to the outer pgx.Tx")
		}
		_, err := momo.Exec(ctx, in, "INSERT INTO adopt_t VALUES (1, 'dup')")
		return err
	})
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("err = %v, want ErrUniqueViolation", err)
	}
	var n int
	if err := ptx.QueryRow(ctx, "SELECT count(*) FROM adopt_t").Scan(&n); err != nil || n != 1 {
		t.Fatalf("legacy count after savepoint rollback = %d, %v", n, err)
	}
}

func TestUnwrapMomoBegunTxThenLegacyStatement(t *testing.T) {
	ctx := context.Background()

	sdb, smdb := adoptSQLDB(t)
	_ = sdb
	err := smdb.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		stx, ok := UnwrapSQLTx(tx)
		if !ok {
			t.Fatal("UnwrapSQLTx false on a FromSQLDB tx")
		}
		if _, ok := UnwrapPgxTx(tx); ok {
			t.Fatal("UnwrapPgxTx must be false on a FromSQLDB tx")
		}
		if _, err := stx.ExecContext(ctx, "INSERT INTO adopt_t VALUES (1, 'legacy')"); err != nil {
			return err
		}
		got, err := momo.Scalars[string](ctx, tx, "SELECT name FROM adopt_t WHERE id = 1 FOR UPDATE")
		if err != nil || len(got) != 1 {
			t.Fatalf("momo sees legacy row: %v %v", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, smdb); n != 1 {
		t.Fatalf("rows = %d, want 1 (momo committed)", n)
	}

	_, pmdb := adoptPgxDB(t)
	err = pmdb.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		ptx, ok := UnwrapPgxTx(tx)
		if !ok {
			t.Fatal("UnwrapPgxTx false on a pool tx")
		}
		if _, ok := UnwrapSQLTx(tx); ok {
			t.Fatal("UnwrapSQLTx must be false on a pool tx")
		}
		if _, err := ptx.Exec(ctx, "INSERT INTO adopt_t VALUES (1, 'legacy')"); err != nil {
			return err
		}
		got, err := momo.Scalars[string](ctx, tx, "SELECT name FROM adopt_t WHERE id = 1 FOR UPDATE")
		if err != nil || len(got) != 1 {
			t.Fatalf("momo sees legacy row: %v %v", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pmdb); n != 1 {
		t.Fatalf("rows = %d, want 1 (momo committed)", n)
	}
}

func TestAdoptPgxTxVectorRoundTripTextMode(t *testing.T) {
	ctx := context.Background()
	pool, mdb := adoptPgxDB(t)
	if _, err := momo.Exec(ctx, mdb, "CREATE TABLE adopt_vec (id int PRIMARY KEY, emb vector(3))"); err != nil {
		t.Fatal(err)
	}
	ptx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptx.Rollback(ctx) }()
	mtx := mdb.AdoptTx(PgxTx(ptx)) // default: Exec mode, VectorText
	if _, err := momo.Exec(ctx, mtx, "INSERT INTO adopt_vec VALUES (?, ?)", 1, vector.Vector{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	got, err := momo.Scalars[string](ctx, mtx, "SELECT emb::text FROM adopt_vec WHERE id = 1")
	if err != nil || len(got) != 1 || got[0] != "[1,2,3]" {
		t.Fatalf("vector round trip = %v, %v", got, err)
	}
}

func TestAdoptPgxTxVectorBinaryDescribeExecOptions(t *testing.T) {
	ctx := context.Background()
	pool := openCodecPool(t, testdb.PostgresURL(t))
	mdb := momo.New(New(pool))
	if _, err := momo.Exec(ctx, mdb, "CREATE TABLE adopt_vecb (id int PRIMARY KEY, emb vector(3))"); err != nil {
		t.Fatal(err)
	}
	ptx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptx.Rollback(ctx) }()
	mtx := mdb.AdoptTx(PgxTx(ptx, WithVectorFormat(VectorBinary), WithExecMode(pgx.QueryExecModeDescribeExec)))
	if _, err := momo.Exec(ctx, mtx, "INSERT INTO adopt_vecb VALUES (?, ?)", 1, vector.Vector{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	got, err := momo.Scalars[vector.Vector](ctx, mtx, "SELECT emb FROM adopt_vecb WHERE id = 1")
	if err != nil || len(got) != 1 || len(got[0]) != 3 || got[0][2] != 3 {
		t.Fatalf("binary vector round trip = %v, %v", got, err)
	}
}

func TestAdoptPgxTxBatchPath(t *testing.T) {
	ctx := context.Background()
	pool, mdb := adoptPgxDB(t)
	ptx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptx.Rollback(ctx) }()
	mtx := mdb.AdoptTx(PgxTx(ptx))
	b, ok := momo.BatcherOf(mtx)
	if !ok {
		t.Fatal("an adopted pgx.Tx must be a Batcher")
	}
	rows, err := b.SendBatch(ctx, []momo.Compiled{
		{SQL: "INSERT INTO adopt_t VALUES ($1, $2)", Args: []any{1, "a"}},
		{SQL: "SELECT name FROM adopt_t WHERE id = $1", Args: []any{1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		_ = r.Close()
	}
	var name string
	if err := ptx.QueryRow(ctx, "SELECT name FROM adopt_t WHERE id = 1").Scan(&name); err != nil || name != "a" {
		t.Fatalf("legacy read after batch = %q, %v", name, err)
	}
}

func TestAdoptPgxTxSavepointAfterCallerRollback(t *testing.T) {
	ctx := context.Background()
	pool, mdb := adoptPgxDB(t)
	ptx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mtx := mdb.AdoptTx(PgxTx(ptx))
	if err := ptx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	called := false
	err = mtx.Tx(ctx, func(*momo.Tx) error { called = true; return nil })
	if !errors.Is(err, momo.ErrTxDone) || called {
		t.Fatalf("savepoint after caller rollback: err=%v called=%v, want ErrTxDone without running fn", err, called)
	}
}
