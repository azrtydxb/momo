// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/examples/internal/exampletest"
	"github.com/azrtydxb/momo/examples/testing/models"
	"github.com/azrtydxb/momo/momotest"
)

// TestExample runs the program itself, as every example does.
func TestExample(t *testing.T) {
	exampletest.CheckGenerated(t)
	db := exampletest.Open(t)
	var out bytes.Buffer
	if err := run(context.Background(), db, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "ada=70\ngrace=80\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// The rest is what the example is about. A test starts from a database that
// has the project's migrations applied (momotest.WithMigrations) and seeds the
// rows it needs.

// seedBank inserts the two accounts every test below starts from and returns
// their owner names. The owners carry the test's tag: parallel tests in
// NewTx mode each hold an open transaction, and two that insert the same
// unique key wait for each other until one ends.
func seedBank(t *testing.T, q momo.Querier, tag string) (ada, grace string) {
	t.Helper()
	a := &models.Account{Owner: "ada-" + tag, Balance: 100}
	g := &models.Account{Owner: "grace-" + tag, Balance: 50}
	momotest.Seed(t, q, momotest.Insert(a, g))
	if a.ID == 0 || g.ID == 0 {
		t.Fatal("Insert did not read the generated ids back")
	}
	return a.Owner, g.Owner
}

// TestTransferRollsBackPerTest uses NewTx: the fast mode. The test's writes
// live in a transaction that is rolled back when it ends, so the parallel
// subtests below start from the same two accounts without seeing each
// other's transfers, and nothing is left behind.
func TestTransferRollsBackPerTest(t *testing.T) {
	for _, amount := range []int64{10, 20, 30, 40} {
		t.Run(fmt.Sprint("amount ", amount), func(t *testing.T) {
			t.Parallel()
			tx := momotest.NewTx(t, momotest.WithMigrations(Migrations()))
			ada, grace := seedBank(t, tx, fmt.Sprint(amount))
			ctx := t.Context()

			if err := Transfer(ctx, tx, ada, grace, amount); err != nil {
				t.Fatal(err)
			}
			if got, err := Balance(ctx, tx, ada); err != nil || got != 100-amount {
				t.Fatalf("ada = %d, %v; want %d", got, err, 100-amount)
			}
		})
	}
}

// TestOverdraftIsRefused checks the database rule through the real
// constraint, not a mock. The failing transfer runs in a savepoint, so the
// transaction stays usable and the test can look at the balances afterwards.
func TestOverdraftIsRefused(t *testing.T) {
	t.Parallel()
	tx := momotest.NewTx(t, momotest.WithMigrations(Migrations()))
	ada, grace := seedBank(t, tx, "overdraft")
	ctx := t.Context()

	err := tx.Tx(ctx, func(sp *momo.Tx) error { return Transfer(ctx, sp, ada, grace, 500) })
	if !errors.Is(err, momo.ErrCheckViolation) {
		t.Fatalf("overdraft = %v, want ErrCheckViolation", err)
	}
	if got, _ := Balance(ctx, tx, grace); got != 50 {
		t.Fatalf("grace = %d after a refused transfer, want 50", got)
	}
}

// TestCommitsForReal uses New: a schema private to the test, dropped when it
// ends. Use this mode when the code under test opens and commits its own
// transactions, as Transfer's caller does here.
func TestCommitsForReal(t *testing.T) {
	t.Parallel()
	db := momotest.New(t, momotest.WithMigrations(Migrations()))
	ada, grace := seedBank(t, db, "commit")
	ctx := t.Context()

	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		return Transfer(ctx, tx, ada, grace, 25)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Balance(ctx, db, grace); err != nil || got != 75 {
		t.Fatalf("grace = %d, %v; want 75 after the committed transfer", got, err)
	}
}
