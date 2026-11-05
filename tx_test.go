// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// A savepoint scope shares the done state of its transaction: once the outer
// transaction has ended, an inner handle that escaped the callback is dead.
func TestSavepointScopeSharesDoneState(t *testing.T) {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.Postgres))
	var inner *momo.Tx
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		return tx.Tx(ctx, func(in *momo.Tx) error {
			inner = in
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inner.Savepoint(ctx, "late"); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("inner Tx after the outer commit: err = %v, want ErrTxDone", err)
	}
}
