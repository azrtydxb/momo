// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func adopted(t *testing.T) (*fakedriver.Driver, momo.DriverTx, *momo.Tx) {
	t.Helper()
	drv := fakedriver.New(dialect.Postgres)
	dtx, err := drv.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return drv, dtx, momo.New(drv).AdoptTx(dtx)
}

func TestAdoptTxCommitRollbackLeaveTransactionToCaller(t *testing.T) {
	drv, _, tx := adopted(t)
	ctx := t.Context()
	if err := tx.Commit(ctx); !errors.Is(err, momo.ErrAdoptedTx) {
		t.Fatalf("Commit = %v, want ErrAdoptedTx", err)
	}
	if err := tx.Rollback(ctx); !errors.Is(err, momo.ErrAdoptedTx) {
		t.Fatalf("Rollback = %v, want ErrAdoptedTx", err)
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 0 {
		t.Fatalf("driver saw commits=%d rollbacks=%d, want none", drv.Commits(), drv.Rollbacks())
	}
	// Refusing is not finishing: a second call refuses the same way.
	if err := tx.Commit(ctx); !errors.Is(err, momo.ErrAdoptedTx) {
		t.Fatalf("second Commit = %v", err)
	}
}

func TestAdoptTxSavepointsWork(t *testing.T) {
	drv, _, tx := adopted(t)
	ctx := t.Context()
	err := tx.Tx(ctx, func(inner *momo.Tx) error {
		return inner.Tx(ctx, func(*momo.Tx) error { return errors.New("boom") })
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
	var sqls []string
	for _, c := range drv.Calls() {
		sqls = append(sqls, c.SQL)
	}
	got := strings.Join(sqls, "|")
	for _, want := range []string{`SAVEPOINT "momo_sp_1"`, `SAVEPOINT "momo_sp_2"`, `ROLLBACK TO SAVEPOINT "momo_sp_2"`, `ROLLBACK TO SAVEPOINT "momo_sp_1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %v", want, sqls)
		}
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 0 {
		t.Fatal("savepoints must not finish the adopted transaction")
	}
	// A savepoint scope keeps its own Commit refusal, not the adopted one.
	_ = tx.Tx(ctx, func(inner *momo.Tx) error {
		if err := inner.Commit(ctx); err == nil || errors.Is(err, momo.ErrAdoptedTx) {
			t.Errorf("savepoint Commit = %v", err)
		}
		return nil
	})
}

func TestAdoptTxDriverTxAccessor(t *testing.T) {
	_, dtx, tx := adopted(t)
	if tx.DriverTx() != dtx {
		t.Fatal("DriverTx did not return the adopted transaction")
	}
	_ = tx.Tx(t.Context(), func(inner *momo.Tx) error {
		if inner.DriverTx() != dtx {
			t.Error("savepoint scope must expose the outer transaction")
		}
		return nil
	})
	db := momo.New(fakedriver.New(dialect.Postgres))
	b, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil || b.DriverTx() == nil {
		t.Fatalf("Begin: %v %v", b, err)
	}
}

func TestAdoptTxNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("AdoptTx(nil) must panic")
		}
	}()
	momo.New(fakedriver.New(dialect.Postgres)).AdoptTx(nil)
}

func TestBegunTxStillCommits(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	tx, err := momo.New(drv).Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil || drv.Commits() != 1 {
		t.Fatalf("Commit = %v commits=%d", err, drv.Commits())
	}
}
