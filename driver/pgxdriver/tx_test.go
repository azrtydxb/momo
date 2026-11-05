// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

func TestTransactionsIsolationReachesServer(t *testing.T) {
	d := New(openPool(t))
	tx, err := d.Begin(context.Background(), momo.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(context.Background(), "SHOW transaction_isolation", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	rows.Next()
	var lvl string
	if err := rows.Scan(&lvl); err != nil {
		t.Fatal(err)
	}
	if lvl != "repeatable read" {
		t.Fatalf("transaction_isolation = %q", lvl)
	}
}

func TestUnknownIsolationIsUnsupported(t *testing.T) {
	d := New(openPool(t))
	_, err := d.Begin(context.Background(), momo.TxOptions{Isolation: sql.IsolationLevel(99)})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// showTx reads one server setting inside tx.
func showTx(t *testing.T, tx momo.DriverTx, setting string) string {
	t.Helper()
	rows, err := tx.Query(context.Background(), "SHOW "+setting, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("SHOW %s returned no row: %v", setting, rows.Err())
	}
	var v string
	if err := rows.Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTransactionsEveryIsolationLevelAndAccessMode(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	cases := []struct {
		lvl  sql.IsolationLevel
		want string
	}{
		{sql.LevelDefault, "read committed"},
		{sql.LevelReadUncommitted, "read uncommitted"},
		{sql.LevelReadCommitted, "read committed"},
		{sql.LevelRepeatableRead, "repeatable read"},
		{sql.LevelSerializable, "serializable"},
	}
	for _, c := range cases {
		for _, ro := range []bool{false, true} {
			tx, err := d.Begin(ctx, momo.TxOptions{Isolation: c.lvl, ReadOnly: ro})
			if err != nil {
				t.Fatalf("level %v: %v", c.lvl, err)
			}
			if got := showTx(t, tx, "transaction_isolation"); got != c.want {
				t.Errorf("level %v: transaction_isolation = %q, want %q", c.lvl, got, c.want)
			}
			wantRO := "off"
			if ro {
				wantRO = "on"
			}
			if got := showTx(t, tx, "transaction_read_only"); got != wantRO {
				t.Errorf("level %v readOnly=%v: transaction_read_only = %q", c.lvl, ro, got)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func createSavepointTable(t *testing.T, d momo.Driver) {
	t.Helper()
	if _, err := d.Exec(context.Background(), "CREATE TABLE sp_items (id int PRIMARY KEY, note text)", nil); err != nil {
		t.Fatal(err)
	}
}

// countItems counts the rows visible to q. A SELECT run through Exec reports
// its row count as the affected-rows number.
func countItems(t *testing.T, q momo.Querier) int {
	t.Helper()
	n, err := momo.Exec(context.Background(), q, "SELECT 1 FROM sp_items")
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

// A failing inner savepoint scope rolls back only its own work: the outer
// transaction stays usable and commits what was done outside the scope.
func TestNestedSavepointRollbackKeepsOuterTxUsable(t *testing.T) {
	d := New(openPool(t))
	createSavepointTable(t, d)
	db := momo.New(d)
	ctx := context.Background()
	boom := errors.New("boom")
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Exec(ctx, tx, "INSERT INTO sp_items VALUES (1, 'outer')"); err != nil {
			return err
		}
		inner := tx.Tx(ctx, func(tx *momo.Tx) error {
			if _, err := momo.Exec(ctx, tx, "INSERT INTO sp_items VALUES (2, 'inner')"); err != nil {
				return err
			}
			// A server error aborts the whole transaction until the savepoint is rolled back.
			if _, err := momo.Exec(ctx, tx, "INSERT INTO sp_items VALUES (2, 'dup')"); err == nil {
				t.Error("duplicate key insert succeeded")
			}
			return boom
		})
		if !errors.Is(inner, boom) {
			t.Errorf("inner err = %v, want boom", inner)
		}
		if _, err := momo.Exec(ctx, tx, "INSERT INTO sp_items VALUES (3, 'outer again')"); err != nil {
			t.Errorf("outer transaction unusable after rollback to savepoint: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countItems(t, db); n != 2 {
		t.Fatalf("committed rows = %d, want 2 (outer insert and the one after the scope)", n)
	}
}

// Two nesting levels: the inner scope succeeds and releases, the middle one
// then fails and discards the inner scope's work with it.
func TestDeepSavepointsUseDistinctGeneratedNames(t *testing.T) {
	d := New(openPool(t))
	createSavepointTable(t, d)
	db := momo.New(d)
	ctx := context.Background()
	boom := errors.New("boom")
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		mid := tx.Tx(ctx, func(tx *momo.Tx) error {
			if err := tx.Tx(ctx, func(tx *momo.Tx) error {
				_, err := momo.Exec(ctx, tx, "INSERT INTO sp_items VALUES (1, 'deep')")
				return err
			}); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(mid, boom) {
			t.Errorf("middle err = %v", mid)
		}
		if n := countItems(t, tx); n != 0 {
			t.Errorf("rows inside tx after discarding the middle scope = %d, want 0", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Savepoint names always reach the server as quoted identifiers, so a hostile
// name cannot inject a statement.
func TestSavepointNameIsQuoted(t *testing.T) {
	d := New(openPool(t))
	createSavepointTable(t, d)
	db := momo.New(d)
	ctx := context.Background()
	hostile := `sp"; DROP TABLE sp_items; --`
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := tx.Savepoint(ctx, hostile); err != nil {
			return err
		}
		if _, err := momo.Exec(ctx, tx, "INSERT INTO sp_items VALUES (1, 'x')"); err != nil {
			return err
		}
		if err := tx.RollbackTo(ctx, hostile); err != nil {
			return err
		}
		return tx.Release(ctx, hostile)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countItems(t, db); n != 0 {
		t.Fatalf("rows = %d, want 0: rollback to the savepoint did not undo the insert", n)
	}
}

func TestSavepointAfterFinishIsTxDone(t *testing.T) {
	d := New(openPool(t))
	db := momo.New(d)
	ctx := context.Background()
	tx, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Savepoint(ctx, "late"); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("Savepoint after Commit err = %v, want ErrTxDone", err)
	}
}
