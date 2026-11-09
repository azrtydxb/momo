// SPDX-License-Identifier: Apache-2.0

package vecsearch_test

import (
	"context"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testmodels/example"
	"github.com/azrtydxb/momo/internal/testmodels/example/document"
	"github.com/azrtydxb/momo/vecsearch"
	"github.com/azrtydxb/momo/vector"
)

// Settings are applied with set_config(..., is_local = true), so they end with
// the transaction and never reach a pooled connection.
func ExampleWith() {
	var db *momo.DB // opened with momo.New(pgxdriver.New(pool))
	ctx := context.Background()

	settings := vecsearch.Settings{
		HNSWEfSearch:      100,
		HNSWIterativeScan: vecsearch.IterativeRelaxedOrder, // needs pgvector 0.8
		HNSWMaxScanTuples: 20000,
	}
	err := vecsearch.With(ctx, db, settings, func(tx *momo.Tx) error {
		_, err := momo.Select[example.Document](tx).
			OrderBy(momo.CosineDistance(document.Embedding, make(vector.Vector, 1536)).Asc()).
			Limit(10).
			All(ctx)
		return err
	})
	fmt.Println(err)
}

// ExactSearch bypasses the vector index for the rest of the transaction so
// the result is the exact nearest neighbours.
func ExampleApply() {
	var db *momo.DB
	ctx := context.Background()

	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := vecsearch.Apply(ctx, tx, vecsearch.Settings{ExactSearch: true}); err != nil {
			return err
		}
		// Queries here scan the table. Keep this transaction short: the
		// planner switches also affect every other index lookup in it.
		return nil
	})
	fmt.Println(err)
}
