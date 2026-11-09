// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// replicaStubDriver is a Driver that returns no rows and affects none.
type replicaStubDriver struct{}

type replicaStubRows struct{}

func (replicaStubRows) Next() bool        { return false }
func (replicaStubRows) Scan(...any) error { return nil }
func (replicaStubRows) Err() error        { return nil }
func (replicaStubRows) Close() error      { return nil }

func (replicaStubDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }

func (replicaStubDriver) Query(context.Context, string, []any) (Rows, error) {
	return replicaStubRows{}, nil
}
func (replicaStubDriver) Exec(context.Context, string, []any) (int64, error) { return 0, nil }
func (d replicaStubDriver) Begin(context.Context, TxOptions) (DriverTx, error) {
	return replicaStubTx{d}, nil
}

type replicaStubTx struct{ replicaStubDriver }

func (replicaStubTx) Commit(context.Context) error   { return nil }
func (replicaStubTx) Rollback(context.Context) error { return nil }

type countingDriver struct {
	Driver
	reads  *int
	writes *int
}

func (c countingDriver) Query(ctx context.Context, sql string, args []any) (Rows, error) {
	*c.reads++
	return c.Driver.Query(ctx, sql, args)
}

func (c countingDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	if c.writes != nil {
		*c.writes++
	}
	return c.Driver.Exec(ctx, sql, args)
}

func readOnce(ctx context.Context, t *testing.T, e *execer) {
	t.Helper()
	rows, err := e.q.Query(ctx, "SELECT 1", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
}

func TestReadReplicaRouting(t *testing.T) {
	var pReads, pWrites, rReads int
	prim := countingDriver{Driver: replicaStubDriver{}, reads: &pReads, writes: &pWrites}
	rep := countingDriver{Driver: replicaStubDriver{}, reads: &rReads}
	db := New(prim, WithReplicas(rep))
	rd, err := db.Replica(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("read without a unit of work uses the replica", func(t *testing.T) {
		before := rReads
		readOnce(ctx, t, rd.conn())
		if rReads != before+1 {
			t.Fatalf("replica reads %d -> %d", before, rReads)
		}
	})

	t.Run("read after a write in the same unit of work uses the primary", func(t *testing.T) {
		uow := NewUnitOfWork(ctx)
		if _, err := Exec(uow, db, "INSERT INTO t VALUES (1)"); err != nil {
			t.Fatal(err)
		}
		beforeR, beforeP := rReads, pReads
		readOnce(uow, t, rd.conn())
		if rReads != beforeR || pReads != beforeP+1 {
			t.Fatalf("replica reads %d -> %d, primary reads %d -> %d", beforeR, rReads, beforeP, pReads)
		}
	})

	t.Run("AllowStaleReads opts out after a write", func(t *testing.T) {
		uow := AllowStaleReads(NewUnitOfWork(ctx))
		if _, err := Exec(uow, db, "INSERT INTO t VALUES (2)"); err != nil {
			t.Fatal(err)
		}
		before := rReads
		readOnce(uow, t, rd.conn())
		if rReads != before+1 {
			t.Fatalf("stale read did not reach the replica: %d -> %d", before, rReads)
		}
	})

	t.Run("writes through the replica handle go to the primary", func(t *testing.T) {
		before := pWrites
		if _, err := Exec(ctx, rd, "INSERT INTO t VALUES (3)"); err != nil {
			t.Fatal(err)
		}
		if pWrites != before+1 {
			t.Fatalf("primary writes %d -> %d", before, pWrites)
		}
	})

	t.Run("transactions never read from the replica", func(t *testing.T) {
		before := rReads
		err := db.Tx(ctx, TxOptions{}, func(tx *Tx) error {
			rows, err := tx.conn().q.Query(ctx, "SELECT 2", nil)
			if err != nil {
				return err
			}
			return rows.Close()
		})
		if err != nil {
			t.Fatal(err)
		}
		if rReads != before {
			t.Fatalf("a read inside a transaction reached the replica")
		}
	})
}

func TestReplicaUnsupportedWithoutReplicas(t *testing.T) {
	if _, err := New(replicaStubDriver{}).Replica(0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// TestReplicaRoutesWritesAndLocksToPrimary: a Replica handle used without any
// unit of work still sends every statement that is not provably a plain read
// to the primary, because the replica is read-only.
func TestReplicaRoutesWritesAndLocksToPrimary(t *testing.T) {
	var pReads, rReads int
	prim := countingDriver{Driver: replicaStubDriver{}, reads: &pReads}
	rep := countingDriver{Driver: replicaStubDriver{}, reads: &rReads}
	rd, err := New(prim, WithReplicas(rep)).Replica(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	primary := []string{
		`INSERT INTO t (a) VALUES ($1) RETURNING id`,
		`UPDATE t SET a = $1 WHERE id = $2 RETURNING id`,
		`DELETE FROM t WHERE id = $1 RETURNING id`,
		`SELECT id FROM t WHERE id = $1 FOR UPDATE`,
		`SELECT id FROM t FOR SHARE`,
		`SELECT id FROM t FOR NO KEY UPDATE SKIP LOCKED`,
		`SELECT id FROM t FOR KEY SHARE`,
		`WITH ins AS (INSERT INTO t (a) VALUES (1) RETURNING id) SELECT id FROM ins`,
		`WITH d AS (DELETE FROM t RETURNING id) SELECT count(*) FROM d`,
		`SELECT nextval('t_id_seq')`,
		`select NEXTVAL('s')`,
		`SELECT set_config('app.tenant', $1, true)`,
		`SELECT pg_advisory_xact_lock(1)`,
		`SELECT * INTO u FROM t`,
		`/* hint */ INSERT INTO t DEFAULT VALUES RETURNING id`,
		`SELECT $q$ x $q$`,
		`CALL do_work()`,
		``,
		// Not a plain read, whatever it starts with.
		`EXPLAIN ANALYZE CREATE TABLE x AS SELECT 1`,
		`EXPLAIN (ANALYZE, VERBOSE) REFRESH MATERIALIZED VIEW v`,
		`EXPLAIN ANALYZE EXECUTE p`,
		`EXPLAIN INSERT INTO t VALUES (1)`,
		`EXPLAIN`,
		`SELECT 1; DROP TABLE t`,
		`SELECT 1; SELECT 2`,
		`CREATE TABLE x (a int)`,
		`DROP TABLE x`,
		`ALTER TABLE x ADD COLUMN b int`,
		`TRUNCATE x`,
		`REFRESH MATERIALIZED VIEW v`,
		`EXECUTE p`,
		`LISTEN ch`,
		`NOTIFY ch`,
		`LOCK TABLE t`,
		`DECLARE c CURSOR FOR SELECT 1`,
		`GRANT SELECT ON t TO u`,
		`REVOKE SELECT ON t FROM u`,
		`VACUUM t`,
		`ANALYZE t`,
		`CLUSTER t`,
		`REINDEX TABLE t`,
		`COMMENT ON TABLE t IS 'x'`,
		`SECURITY LABEL ON TABLE t IS 'x'`,
		`IMPORT FOREIGN SCHEMA s FROM SERVER v INTO d`,
		`SET search_path = x`,
		`SELECT lo_import('/etc/passwd')`,
		`SELECT lo_export(1, '/tmp/x')`,
		`SELECT lo_unlink(1)`,
		`SELECT dblink_exec('c', 'DELETE FROM t')`,
		`SELECT pg_reload_conf()`,
		`SELECT pg_switch_wal()`,
		`SELECT pg_logical_emit_message(true, 'p', 'm')`,
		`SELECT pg_terminate_backend(1)`,
		`SELECT pg_cancel_backend(1)`,
		`SELECT 'unterminated`,
		`SELECT 1 /* unterminated`,
	}
	for _, q := range primary {
		bp, br := pReads, rReads
		readOnceSQL(ctx, t, rd.conn(), q)
		if pReads != bp+1 || rReads != br {
			t.Errorf("%q must run on the primary: primary %d -> %d, replica %d -> %d", q, bp, pReads, br, rReads)
		}
	}
	replica := []string{
		`SELECT id FROM t WHERE id = $1`,
		`select "update", 'for update' from t -- INSERT`,
		`WITH x AS (SELECT 1) SELECT * FROM x`,
		`/* INSERT */ SELECT 1`,
		`(SELECT 1) UNION (SELECT 2)`,
		`VALUES (1), (2)`,
		`SELECT substring(a FROM 1 FOR 2) FROM t`,
		`EXPLAIN SELECT 1`,
		`SELECT id FROM t ORDER BY embedding <=> $1 LIMIT 5`,
		`EXPLAIN (ANALYZE, VERBOSE) SELECT 1`,
		`EXPLAIN ANALYZE VERBOSE SELECT 1`,
		`TABLE t`,
		`SELECT 1;`,
		`SELECT 1; -- trailing comment`,
		`SELECT "do", "comment" FROM "create"`,
	}
	for _, q := range replica {
		bp, br := pReads, rReads
		readOnceSQL(ctx, t, rd.conn(), q)
		if rReads != br+1 || pReads != bp {
			t.Errorf("%q must run on the replica: primary %d -> %d, replica %d -> %d", q, bp, pReads, br, rReads)
		}
	}
}

func readOnceSQL(ctx context.Context, t *testing.T, e *execer, sql string) {
	t.Helper()
	rows, err := e.q.Query(ctx, sql, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
}

type failingReplica struct {
	replicaStubDriver
	err   error
	calls *int
}

func (f failingReplica) Query(context.Context, string, []any) (Rows, error) {
	*f.calls++
	return nil, f.err
}

type stateErr struct{ code string }

func (e stateErr) Error() string    { return "sqlstate " + e.code }
func (e stateErr) SQLState() string { return e.code }

// TestReplicaFailureFallsBackToPrimary: a connection-class error from the
// replica when a read starts re-runs the read on the primary; other errors,
// and a cancelled caller context, are returned as they are.
func TestReplicaFailureFallsBackToPrimary(t *testing.T) {
	down := []error{
		&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
		fmt.Errorf("acquire: %w", syscall.ECONNRESET),
		stateErr{"08006"},
		stateErr{"57P01"},
		stateErr{"53300"},
	}
	for _, cause := range down {
		var pReads, rCalls int
		prim := countingDriver{Driver: replicaStubDriver{}, reads: &pReads}
		rd, err := New(prim, WithReplicas(failingReplica{err: cause, calls: &rCalls})).Replica(0)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := rd.conn().q.Query(context.Background(), "SELECT 1", nil)
		if err != nil {
			t.Fatalf("%v: want fallback to the primary, got %v", cause, err)
		}
		_ = rows.Close()
		if rCalls != 1 || pReads != 1 {
			t.Fatalf("%v: replica calls %d, primary reads %d, want 1 and 1", cause, rCalls, pReads)
		}
	}

	t.Run("an error that is not connection-class is returned", func(t *testing.T) {
		var pReads, rCalls int
		boom := stateErr{"42601"} // syntax error
		prim := countingDriver{Driver: replicaStubDriver{}, reads: &pReads}
		rd, _ := New(prim, WithReplicas(failingReplica{err: boom, calls: &rCalls})).Replica(0)
		if _, err := rd.conn().q.Query(context.Background(), "SELECT 1", nil); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the replica's error", err)
		}
		if pReads != 0 {
			t.Fatal("a non-connection error must not be retried on the primary")
		}
	})

	t.Run("a cancelled caller context is not retried", func(t *testing.T) {
		var pReads, rCalls int
		prim := countingDriver{Driver: replicaStubDriver{}, reads: &pReads}
		rd, _ := New(prim, WithReplicas(failingReplica{err: context.Canceled, calls: &rCalls})).Replica(0)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := rd.conn().q.Query(ctx, "SELECT 1", nil); err == nil {
			t.Fatal("want the cancellation error")
		}
		if pReads != 0 {
			t.Fatal("a cancelled context must not be retried on the primary")
		}
	})
}

// A statement that goes to the primary because it is not a plain read counts
// as a write of the unit of work, so later reads see its effects.
func TestReplicaNonReadMarksUnitOfWork(t *testing.T) {
	var pReads, rReads int
	prim := countingDriver{Driver: replicaStubDriver{}, reads: &pReads}
	rep := countingDriver{Driver: replicaStubDriver{}, reads: &rReads}
	rd, _ := New(prim, WithReplicas(rep)).Replica(0)
	uow := NewUnitOfWork(context.Background())
	readOnceSQL(uow, t, rd.conn(), `SELECT nextval('s')`)
	br := rReads
	readOnceSQL(uow, t, rd.conn(), `SELECT 1`)
	if rReads != br {
		t.Fatal("a read after SELECT nextval in the same unit of work must use the primary")
	}
}

// WithImplicitTx runs every statement on the primary, so a Replica handle is
// refused instead of silently never using the replica.
func TestReplicaRefusedWithImplicitTx(t *testing.T) {
	db := New(replicaStubDriver{}, WithReplicas(replicaStubDriver{}), WithImplicitTx())
	if _, err := db.Replica(0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// A raw read through Scalars is routed by its own SQL text and does not mark
// the unit of work as written, so later reads may still use a replica.
func TestReplicaScalarsReadDoesNotMarkUnitOfWork(t *testing.T) {
	var pReads, rReads int
	prim := countingDriver{Driver: replicaStubDriver{}, reads: &pReads}
	rep := countingDriver{Driver: replicaStubDriver{}, reads: &rReads}
	rd, _ := New(prim, WithReplicas(rep)).Replica(0)
	uow := NewUnitOfWork(context.Background())
	if _, err := Scalars[int64](uow, rd, `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	before := rReads
	readOnceSQL(uow, t, rd.conn(), `SELECT 2`)
	if rReads != before+1 {
		t.Fatal("a Scalars read must not force later reads of the unit of work to the primary")
	}
}
