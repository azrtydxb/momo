// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
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
