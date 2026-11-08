// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlitedriver

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
)

// TestSQLiteReadOnlyCommitFailureDiscardsConnection: when COMMIT fails and the
// ROLLBACK that follows fails too, the connection's state is unknown, so it
// must not return to the pool.
func TestSQLiteReadOnlyCommitFailureDiscardsConnection(t *testing.T) {
	db, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	tx, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	execStmtHook = func(stmt string) error {
		if stmt == "COMMIT" || stmt == "ROLLBACK" {
			return errors.New("injected failure")
		}
		return nil
	}
	t.Cleanup(func() { execStmtHook = nil })
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("Commit succeeded although the statement failed")
	}
	execStmtHook = nil
	if st := db.Stats(); st.OpenConnections != 0 {
		t.Fatalf("OpenConnections = %d; the suspect connection was returned to the pool", st.OpenConnections)
	}
	// The pool opens a fresh, healthy connection for the next statement.
	if _, err := drv.Exec(ctx, "CREATE TABLE after_discard (x int)", nil); err != nil {
		t.Fatalf("the pool is unusable after the discard: %v", err)
	}
}

// TestSQLiteReadOnlyCommitFailureRollsBack: a failed COMMIT whose ROLLBACK
// succeeds returns the connection in autocommit mode, query_only reset.
func TestSQLiteReadOnlyCommitFailureRollsBack(t *testing.T) {
	db, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := drv.Exec(ctx, "CREATE TABLE rb (x int)", nil); err != nil {
		t.Fatal(err)
	}
	tx, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	execStmtHook = func(stmt string) error {
		if stmt == "COMMIT" {
			return errors.New("injected failure")
		}
		return nil
	}
	t.Cleanup(func() { execStmtHook = nil })
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("Commit succeeded although the statement failed")
	}
	execStmtHook = nil
	if _, err := drv.Exec(ctx, "INSERT INTO rb VALUES (1)", nil); err != nil {
		t.Fatalf("the connection was returned read-only or inside a transaction: %v", err)
	}
}
