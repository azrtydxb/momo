// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysqldriver_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

func countRows(t *testing.T, q momo.Querier, table string) int64 {
	t.Helper()
	got, err := momo.Scalars[int64](context.Background(), q, "SELECT count(*) FROM "+table)
	if err != nil || len(got) != 1 {
		t.Fatalf("count(%s) = %v, %v", table, got, err)
	}
	return got[0]
}

func TestTransactions(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE t (id bigint PRIMARY KEY)")

			if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				_, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (1)")
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if got := countRows(t, db, "t"); got != 1 {
				t.Fatalf("commit: rows = %d, want 1", got)
			}

			wantErr := errors.New("boom")
			if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (2)"); err != nil {
					return err
				}
				return wantErr
			}); !errors.Is(err, wantErr) {
				t.Fatalf("rollback: err = %v, want boom", err)
			}
			if got := countRows(t, db, "t"); got != 1 {
				t.Fatalf("rollback: rows = %d, want 1", got)
			}

			func() {
				defer func() {
					if r := recover(); r == nil {
						t.Fatal("panic was swallowed")
					}
				}()
				err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
					if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (3)"); err != nil {
						t.Fatal(err)
					}
					panic("callback panic")
				})
				t.Fatalf("Tx returned %v after a panic; the panic must be re-raised", err)
			}()
			if got := countRows(t, db, "t"); got != 1 {
				t.Fatalf("panic: rows = %d, want 1", got)
			}

			if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (4)"); err != nil {
					return err
				}
				if err := tx.Savepoint(ctx, "sp"); err != nil {
					return err
				}
				if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (5)"); err != nil {
					return err
				}
				return tx.RollbackTo(ctx, "sp")
			}); err != nil {
				t.Fatal(err)
			}
			if got := countRows(t, db, "t"); got != 2 {
				t.Fatalf("savepoint: rows = %d, want 2 (ids 1 and 4)", got)
			}

			if err := db.Tx(ctx, momo.TxOptions{ReadOnly: true}, func(tx *momo.Tx) error {
				_, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (6)")
				return err
			}); err == nil {
				t.Fatal("INSERT in a read-only transaction succeeded")
			}

			// Every standard isolation level starts a transaction.
			for _, lvl := range []sql.IsolationLevel{sql.LevelReadCommitted, sql.LevelRepeatableRead, sql.LevelSerializable} {
				if err := db.Tx(ctx, momo.TxOptions{Isolation: lvl}, func(tx *momo.Tx) error {
					_, err := momo.Exec(ctx, tx, "SELECT 1")
					return err
				}); err != nil {
					t.Fatalf("isolation %v: %v", lvl, err)
				}
			}
			if err := db.Tx(ctx, momo.TxOptions{Isolation: sql.LevelSnapshot}, func(*momo.Tx) error { return nil }); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("snapshot isolation: err = %v, want ErrUnsupported", err)
			}
		})
	}
}

// TestMySQLLockSyntax proves the lock clauses Momo renders are accepted by each
// server: a NOWAIT lock on a row held by another transaction fails at once, a
// SKIP LOCKED read skips it, and FOR SHARE parses (as LOCK IN SHARE MODE on MariaDB).
func TestMySQLLockSyntax(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE locks_t (id bigint PRIMARY KEY)")
			mustExecMySQL(t, db, "INSERT INTO locks_t VALUES (1), (2)")
			if err := db.Tx(ctx, momo.TxOptions{}, func(holder *momo.Tx) error {
				if _, err := momo.Scalars[int64](ctx, holder, "SELECT id FROM locks_t WHERE id = 1 FOR UPDATE"); err != nil {
					return err
				}
				return db.Tx(ctx, momo.TxOptions{}, func(other *momo.Tx) error {
					if _, err := momo.Scalars[int64](ctx, other, "SELECT id FROM locks_t WHERE id = 1 FOR UPDATE NOWAIT"); err == nil {
						t.Error("NOWAIT on a locked row succeeded")
					}
					return nil
				})
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Tx(ctx, momo.TxOptions{}, func(holder *momo.Tx) error {
				if _, err := momo.Scalars[int64](ctx, holder, "SELECT id FROM locks_t WHERE id = 1 FOR UPDATE"); err != nil {
					return err
				}
				return db.Tx(ctx, momo.TxOptions{}, func(other *momo.Tx) error {
					got, err := momo.Scalars[int64](ctx, other, "SELECT id FROM locks_t ORDER BY id FOR UPDATE SKIP LOCKED")
					if err != nil || len(got) != 1 || got[0] != 2 {
						t.Errorf("SKIP LOCKED = %v, %v, want [2]", got, err)
					}
					return nil
				})
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type lockRow struct{ ID int64 }

func (r *lockRow) MomoMeta() *momo.Meta[lockRow] {
	return &momo.Meta[lockRow]{
		Table:    "locks_t",
		PK:       []int{0},
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}},
		Ptr:     func(r *lockRow, i int) any { return &r.ID },
		Value:   func(r *lockRow, i int) any { return r.ID },
	}
}

// TestSelectForShareRendersPerFlavor runs the builder's own ForShare, which
// MariaDB must receive as LOCK IN SHARE MODE.
func TestSelectForShareRendersPerFlavor(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE locks_t (id bigint PRIMARY KEY)")
			mustExecMySQL(t, db, "INSERT INTO locks_t VALUES (1)")
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				rows, err := momo.Select[lockRow](tx).ForShare().All(ctx)
				if err != nil || len(rows) != 1 {
					t.Errorf("ForShare = %v, %v", rows, err)
				}
				rows, err = momo.Select[lockRow](tx).ForUpdate("SKIP LOCKED").All(ctx)
				if err != nil || len(rows) != 1 {
					t.Errorf("ForUpdate SKIP LOCKED = %v, %v", rows, err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
