// SPDX-License-Identifier: Apache-2.0

package pgxdriver_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

var errRollback = errors.New("roll back on purpose")

func TestCopyAndBatchInTransaction(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE tx_copy (id bigint, name text)"); err != nil {
		t.Fatal(err)
	}
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		c, ok := momo.CopierOf(tx)
		if !ok {
			return errors.New("transaction is not a Copier")
		}
		n, err := c.CopyFrom(ctx, "tx_copy", []string{"id", "name"}, [][]any{{int64(1), "a"}, {int64(2), "b"}})
		if err != nil {
			return err
		}
		if n != 2 {
			return fmt.Errorf("copied %d rows, want 2", n)
		}
		b, ok := momo.BatcherOf(tx)
		if !ok {
			return errors.New("transaction is not a Batcher")
		}
		rs, err := b.SendBatch(ctx, []momo.Compiled{{SQL: "SELECT count(*) FROM tx_copy"}})
		if err != nil {
			return err
		}
		if len(rs) != 1 {
			return fmt.Errorf("batch returned %d result sets, want 1", len(rs))
		}
		if !rs[0].Next() {
			return errors.New("batch count returned no row")
		}
		var inTx int
		if err := rs[0].Scan(&inTx); err != nil {
			return err
		}
		if err := rs[0].Close(); err != nil {
			return err
		}
		if inTx != 2 {
			return fmt.Errorf("batch inside the transaction saw %d rows, want 2", inTx)
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("Tx err = %v, want the rollback sentinel", err)
	}
	rows, err := db.Driver().Query(ctx, "SELECT count(*) FROM tx_copy", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatalf("count returned no row: %v", rows.Err())
	}
	var after int
	if err := rows.Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("rolled-back COPY left %d rows", after)
	}
}
