// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

var errRollMeBack = errors.New("suite: roll me back")

// count returns the number of documents with the given title.
func count(t *testing.T, q momo.Querier, title string) int64 {
	t.Helper()
	got, err := momo.Scalars[int64](context.Background(), q, "SELECT count(*) FROM documents WHERE title = ?", title)
	if err != nil || len(got) != 1 {
		t.Fatalf("count(%q) = %v, %v", title, got, err)
	}
	return got[0]
}

func insertDoc(ctx context.Context, q momo.Querier, title string) error {
	_, err := momo.Exec(ctx, q, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", 1, title)
	return err
}

// Transactions asserts the transaction behaviour S-7 requires: commit on nil,
// rollback on error, rollback and re-raise on panic, savepoint isolation of a
// failed nested Tx, and isolation levels per caps.
func Transactions(t *testing.T, db *momo.DB, caps Caps) {
	ctx := context.Background()
	CreateDocuments(t, db, caps)

	t.Run("commit", func(t *testing.T) {
		if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error { return insertDoc(ctx, tx, "committed") }); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "committed"); n != 1 {
			t.Fatalf("committed rows = %d, want 1", n)
		}
	})

	t.Run("rollback on error", func(t *testing.T) {
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := insertDoc(ctx, tx, "rolled-back"); err != nil {
				return err
			}
			return errRollMeBack
		})
		if !errors.Is(err, errRollMeBack) {
			t.Fatalf("err = %v, want errRollMeBack", err)
		}
		if n := count(t, db, "rolled-back"); n != 0 {
			t.Fatalf("rolled-back rows = %d, want 0", n)
		}
	})

	t.Run("panic rolls back and re-raises", func(t *testing.T) {
		var recovered any
		func() {
			defer func() { recovered = recover() }()
			_ = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				if err := insertDoc(ctx, tx, "panicked"); err != nil {
					return err
				}
				panic("boom")
			})
		}()
		if recovered == nil {
			t.Fatal("panic was swallowed")
		}
		if n := count(t, db, "panicked"); n != 0 {
			t.Fatalf("panicked rows = %d, want 0", n)
		}
	})

	t.Run("failed savepoint rolls back only its own work", func(t *testing.T) {
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := insertDoc(ctx, tx, "outer"); err != nil {
				return err
			}
			inner := tx.Tx(ctx, func(in *momo.Tx) error {
				if err := insertDoc(ctx, in, "inner"); err != nil {
					return err
				}
				return errRollMeBack
			})
			if !errors.Is(inner, errRollMeBack) {
				t.Errorf("inner err = %v, want errRollMeBack", inner)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "outer"); n != 1 {
			t.Errorf("outer rows = %d, want 1", n)
		}
		if n := count(t, db, "inner"); n != 0 {
			t.Errorf("inner rows = %d, want 0", n)
		}
	})

	t.Run("nested success persists", func(t *testing.T) {
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := insertDoc(ctx, tx, "nested-outer"); err != nil {
				return err
			}
			return tx.Tx(ctx, func(in *momo.Tx) error { return insertDoc(ctx, in, "nested-inner") })
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, title := range []string{"nested-outer", "nested-inner"} {
			if n := count(t, db, title); n != 1 {
				t.Errorf("%s rows = %d, want 1", title, n)
			}
		}
	})

	t.Run("inner panic rolls back to the savepoint and re-raises", func(t *testing.T) {
		var recovered any
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := insertDoc(ctx, tx, "panic-outer"); err != nil {
				return err
			}
			func() {
				defer func() { recovered = recover() }()
				_ = tx.Tx(ctx, func(in *momo.Tx) error {
					if err := insertDoc(ctx, in, "panic-inner"); err != nil {
						return err
					}
					panic("inner boom")
				})
			}()
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if recovered == nil {
			t.Fatal("inner panic was swallowed")
		}
		if n := count(t, db, "panic-outer"); n != 1 {
			t.Errorf("panic-outer rows = %d, want 1 (outer recovered and committed)", n)
		}
		if n := count(t, db, "panic-inner"); n != 0 {
			t.Errorf("panic-inner rows = %d, want 0", n)
		}
	})

	t.Run("isolation", func(t *testing.T) {
		if caps.NonDefaultIsolation {
			err := db.Tx(ctx, momo.TxOptions{Isolation: sql.LevelRepeatableRead}, func(tx *momo.Tx) error {
				got, err := momo.Scalars[string](ctx, tx, "SHOW transaction_isolation")
				if err != nil {
					return err
				}
				if len(got) != 1 || got[0] != "repeatable read" {
					t.Errorf("transaction_isolation = %v, want [repeatable read]", got)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			return
		}
		err := db.Tx(ctx, momo.TxOptions{Isolation: sql.LevelReadUncommitted}, func(*momo.Tx) error { return nil })
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("err = %v, want ErrUnsupported", err)
		}
	})
}
