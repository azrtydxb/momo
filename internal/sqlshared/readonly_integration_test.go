// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlshared_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/internal/sqlshared"
)

// A Rows left open when a read-only transaction ends holds the connection's
// close lock, so releasing the connection would block for good. Commit and
// Rollback close such Rows first.
func TestReadOnlyTxEndsWithOpenRows(t *testing.T) {
	for _, end := range []string{"commit", "rollback"} {
		t.Run(end, func(t *testing.T) {
			eachDriver(t, func(t *testing.T, o opener) {
				db, drv := openAt(t, o, sqlitepure.Config{})
				ctx := context.Background()
				if _, err := drv.Exec(ctx, "CREATE TABLE ro_rows (x int)", nil); err != nil {
					t.Fatal(err)
				}
				if _, err := drv.Exec(ctx, "INSERT INTO ro_rows VALUES (1), (2)", nil); err != nil {
					t.Fatal(err)
				}
				tx, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
				if err != nil {
					t.Fatal(err)
				}
				rows, err := tx.Query(ctx, "SELECT x FROM ro_rows", nil)
				if err != nil {
					t.Fatal(err)
				}
				if !rows.Next() {
					t.Fatalf("no row: %v", rows.Err())
				}
				done := make(chan error, 1)
				go func() {
					if end == "commit" {
						done <- tx.Commit(ctx)
					} else {
						done <- tx.Rollback(ctx)
					}
				}()
				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("%s: %v", end, err)
					}
				case <-time.After(10 * time.Second):
					t.Fatalf("%s blocked: an open Rows kept the connection from being released", end)
				}
				_ = rows.Close()
				if st := db.Stats(); st.InUse != 0 {
					t.Fatalf("InUse = %d; the connection was not returned", st.InUse)
				}
				if _, err := drv.Exec(ctx, "INSERT INTO ro_rows VALUES (3)", nil); err != nil {
					t.Fatalf("the connection stayed read-only: %v", err)
				}
				// Query after the end reports the finished transaction.
				if _, err := tx.Query(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrTxDone) {
					t.Fatalf("Query after %s err = %v, want ErrTxDone", end, err)
				}
			})
		})
	}
}

// If PRAGMA query_only=ON reports an error after it took effect (a context
// cancelled while it ran), the connection must not go back to the pool
// read-only.
func TestReadOnlyBeginFailureResetsQueryOnly(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		_, drv := openAt(t, o, sqlitepure.Config{})
		ctx := context.Background()
		if _, err := drv.Exec(ctx, "CREATE TABLE ro_begin (x int)", nil); err != nil {
			t.Fatal(err)
		}
		sqlshared.AfterBeginStmtHook = func(stmt string) error {
			if stmt == "PRAGMA query_only=ON" {
				return errors.New("injected failure")
			}
			return nil
		}
		t.Cleanup(func() { sqlshared.AfterBeginStmtHook = nil })
		if _, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true}); err == nil {
			t.Fatal("Begin succeeded although the pragma statement reported an error")
		}
		sqlshared.AfterBeginStmtHook = nil
		if _, err := drv.Exec(ctx, "INSERT INTO ro_begin VALUES (1)", nil); err != nil {
			t.Fatalf("the connection went back to the pool read-only: %v", err)
		}
	})
}

// Both drivers classify a duplicate explicit rowid (SQLITE_CONSTRAINT_ROWID)
// as a unique violation, and name the table.
func TestDuplicateRowidIsUniqueViolation(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		_, drv := openAt(t, o, sqlitepure.Config{})
		ctx := context.Background()
		if _, err := drv.Exec(ctx, "CREATE TABLE dup_rowid (x text)", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := drv.Exec(ctx, "INSERT INTO dup_rowid (rowid, x) VALUES (1, 'a')", nil); err != nil {
			t.Fatal(err)
		}
		_, err := drv.Exec(ctx, "INSERT INTO dup_rowid (rowid, x) VALUES (1, 'b')", nil)
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("err = %v, want ErrUniqueViolation", err)
		}
		var me *momo.Error
		if !errors.As(err, &me) || me.Table != "dup_rowid" {
			t.Fatalf("err = %v, want Table dup_rowid", err)
		}
	})
}
