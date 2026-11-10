// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/azrtydxb/momo"
)

func main() {
	var db *momo.DB
	var tx *momo.Tx
	ctx := context.Background()
	s := momo.SearchSettings{HNSWIterativeScan: momo.IterativeRelaxedOrder}
	_ = db.WithSearch(ctx, s, func(*momo.Tx) error { return nil })
	_ = tx.SetSearch(ctx, s)
}
